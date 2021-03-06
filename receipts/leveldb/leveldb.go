package leveldb

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"os"

	"github.com/gogo/protobuf/proto"
	"github.com/diademnetwork/go-diadem"
	"github.com/diademnetwork/go-diadem/plugin/types"
	diadem_types "github.com/diademnetwork/go-diadem/types"
	"github.com/diademnetwork/diademchain"
	"github.com/diademnetwork/diademchain/eth/bdiadem"
	"github.com/diademnetwork/diademchain/log"
	"github.com/diademnetwork/diademchain/receipts/common"
	"github.com/pkg/errors"
	"github.com/syndtr/goleveldb/leveldb"
)

const (
	Db_Filename = "receipts_db"
)

var (
	headKey          = []byte("leveldb:head")
	tailKey          = []byte("leveldb:tail")
	currentDbSizeKey = []byte("leveldb:size")
)

func WriteReceipt(
	block diadem_types.BlockHeader,
	caller, addr diadem.Address,
	events []*types.EventData,
	status int32,
	eventHadler diademchain.EventHandler,
	evmTxIndex int32,
	nonce int64,
) (types.EvmTxReceipt, error) {
	txReceipt := types.EvmTxReceipt{
		Nonce:             nonce,
		TransactionIndex:  evmTxIndex,
		BlockHash:         block.CurrentHash,
		BlockNumber:       block.Height,
		CumulativeGasUsed: 0,
		GasUsed:           0,
		ContractAddress:   addr.Local,
		LogsBdiadem:         bdiadem.GenBdiademFilter(events),
		Status:            status,
		CallerAddress:     caller.MarshalPB(),
	}

	preTxReceipt, err := proto.Marshal(&txReceipt)
	if err != nil {
		return types.EvmTxReceipt{}, errors.Wrapf(err, "marshalling receipt")
	}
	h := sha256.New()
	h.Write(preTxReceipt)
	txHash := h.Sum(nil)

	txReceipt.TxHash = txHash
	blockHeight := uint64(txReceipt.BlockNumber)
	for _, event := range events {
		event.TxHash = txHash
		if eventHadler != nil {
			_ = eventHadler.Post(blockHeight, event)
		}

		pEvent := types.EventData(*event)
		pEvent.BlockHash = block.CurrentHash
		pEvent.TransactionIndex = uint64(evmTxIndex)
		txReceipt.Logs = append(txReceipt.Logs, &pEvent)
	}

	return txReceipt, nil
}

func (lr *LevelDbReceipts) GetReceipt(txHash []byte) (types.EvmTxReceipt, error) {
	txReceiptProto, err := lr.db.Get(txHash, nil)
	if err != nil {
		return types.EvmTxReceipt{}, errors.Wrapf(err, "get receipt for %s", string(txHash))
	}
	txReceipt := types.EvmTxReceiptListItem{}
	err = proto.Unmarshal(txReceiptProto, &txReceipt)
	return *txReceipt.Receipt, err
}

type LevelDbReceipts struct {
	MaxDbSize uint64
	db        *leveldb.DB
	tran      *leveldb.Transaction
}

func NewLevelDbReceipts(maxReceipts uint64) (*LevelDbReceipts, error) {
	db, err := leveldb.OpenFile(Db_Filename, nil)
	if err != nil {
		return nil, errors.New("opening leveldb")
	}
	return &LevelDbReceipts{
		MaxDbSize: maxReceipts,
		db:        db,
		tran:      nil,
	}, nil
}

func (lr LevelDbReceipts) Close() error {
	if lr.db != nil {
		return lr.db.Close()
	}
	return nil
}

func (lr *LevelDbReceipts) CommitBlock(state diademchain.State, receipts []*types.EvmTxReceipt, height uint64) error {
	if len(receipts) == 0 {
		return nil
	}

	size, headHash, tailHash, err := getDBParams(lr.db)
	if err != nil {
		return errors.Wrap(err, "getting db params.")
	}

	lr.tran, err = lr.db.OpenTransaction()
	if err != nil {
		return errors.Wrap(err, "opening leveldb transaction")
	}
	defer lr.closeTransaction()

	tailReceiptItem := types.EvmTxReceiptListItem{}
	if len(headHash) > 0 {
		tailItemProto, err := lr.tran.Get(tailHash, nil)
		if err != nil {
			return errors.Wrap(err, "cannot find tail")
		}
		if err = proto.Unmarshal(tailItemProto, &tailReceiptItem); err != nil {
			return errors.Wrap(err, "unmarshalling tail")
		}
	}

	var txHashArray [][]byte
	events := make([]*types.EventData, 0, len(receipts))
	for _, txReceipt := range receipts {
		if txReceipt == nil || len(txReceipt.TxHash) == 0 {
			continue
		}

		// Update previous tail to point to current receipt
		if len(headHash) == 0 {
			headHash = txReceipt.TxHash
		} else {
			tailReceiptItem.NextTxHash = txReceipt.TxHash
			protoTail, err := proto.Marshal(&tailReceiptItem)
			if err != nil {
				log.Error(fmt.Sprintf("commit block receipts: marshal receipt item: %s", err.Error()))
				continue
			}
			updating, err := lr.tran.Has(tailHash, nil)
			if err != nil {
				return errors.Wrap(err, "cannot find tail hash")
			}

			if err := lr.tran.Put(tailHash, protoTail, nil); err != nil {
				log.Error(fmt.Sprintf("commit block receipts: put receipt in db: %s", err.Error()))
				continue
			} else if !updating {
				size++
			}
		}

		// Set current receipt as next tail
		tailHash = txReceipt.TxHash
		tailReceiptItem = types.EvmTxReceiptListItem{Receipt: txReceipt, NextTxHash: nil}

		// only upload hashes to app db if transaction successful
		if txReceipt.Status == common.StatusTxSuccess {
			txHashArray = append(txHashArray, txReceipt.TxHash)
		}

		events = append(events, txReceipt.Logs...)
	}
	if len(tailHash) > 0 {
		protoTail, err := proto.Marshal(&tailReceiptItem)
		if err != nil {
			log.Error(fmt.Sprintf("commit block receipts: marshal receipt item: %s", err.Error()))
		} else {
			updating, err := lr.tran.Has(tailHash, nil)
			if err != nil {
				return errors.Wrap(err, "cannot find tail hash")
			}
			if err := lr.tran.Put(tailHash, protoTail, nil); err != nil {
				log.Error(fmt.Sprintf("commit block receipts: putting receipt in db: %s", err.Error()))
			} else if !updating {
				size++
			}
		}
	}

	if lr.MaxDbSize < size {
		var numDeleted uint64
		headHash, numDeleted, err = removeOldEntries(lr.tran, headHash, size-lr.MaxDbSize)
		if err != nil {
			return errors.Wrap(err, "removing old receipts")
		}
		if size < numDeleted {
			return errors.Wrap(err, "invalid count of deleted receipts")
		}
		size -= numDeleted
	}
	if err := setDBParams(lr.tran, size, headHash, tailHash); err != nil {
		return errors.Wrap(err, "saving receipt db params")
	}

	if err := common.AppendTxHashList(state, txHashArray, height); err != nil {
		return errors.Wrap(err, "append tx list")
	}
	filter := bdiadem.GenBdiademFilter(events)
	common.SetBdiademFilter(state, filter, height)

	if err := lr.tran.Commit(); err != nil {
		return errors.Wrap(err, "committing level db transaction")
	}
	lr.tran = nil
	return nil
}

func (lr *LevelDbReceipts) ClearData() {
	os.RemoveAll(Db_Filename)
}

func (lr *LevelDbReceipts) closeTransaction() {
	if lr.tran != nil {
		lr.tran.Discard()
		lr.tran = nil
	}
}

func removeOldEntries(tran *leveldb.Transaction, head []byte, number uint64) ([]byte, uint64, error) {
	itemsDeleted := uint64(0)
	for i := uint64(0); i < number && len(head) > 0; i++ {
		headItem, err := tran.Get(head, nil)
		if err != nil {
			return head, itemsDeleted, errors.Wrapf(err, "get head %s", string(head))
		}
		txHeadReceiptItem := types.EvmTxReceiptListItem{}
		if err := proto.Unmarshal(headItem, &txHeadReceiptItem); err != nil {
			return head, itemsDeleted, errors.Wrapf(err, "unmarshal head %s", string(headItem))
		}
		tran.Delete(head, nil)
		itemsDeleted++
		head = txHeadReceiptItem.NextTxHash
	}
	if itemsDeleted < number {
		return head, itemsDeleted, errors.Errorf("Unable to delete %v receipts, only %v deleted", number, itemsDeleted)
	}

	return head, itemsDeleted, nil
}

func getDBParams(db *leveldb.DB) (size uint64, head, tail []byte, err error) {
	notEmpty, err := db.Has(currentDbSizeKey, nil)
	if err != nil {
		return size, head, tail, err
	}
	if !notEmpty {
		return 0, []byte{}, []byte{}, nil
	}

	sizeB, err := db.Get(currentDbSizeKey, nil)
	if err != nil {
		return size, head, tail, err
	}
	size = binary.LittleEndian.Uint64(sizeB)
	if size == 0 {
		return 0, []byte{}, []byte{}, nil
	}

	head, err = db.Get(headKey, nil)
	if err != nil {
		return size, head, tail, err
	}
	if len(head) == 0 {
		return 0, []byte{}, []byte{}, errors.New("no head for non zero size receipt db")
	}

	tail, err = db.Get(tailKey, nil)
	if err != nil {
		return size, head, tail, err
	}
	if len(tail) == 0 {
		return 0, []byte{}, []byte{}, errors.New("no tail for non zero size receipt db")
	}

	return size, head, tail, nil
}

func setDBParams(tr *leveldb.Transaction, size uint64, head, tail []byte) error {
	if err := tr.Put(headKey, head, nil); err != nil {
		return err
	}

	if err := tr.Put(tailKey, tail, nil); err != nil {
		return err
	}

	sizeB := make([]byte, 8)
	binary.LittleEndian.PutUint64(sizeB, size)
	return tr.Put(currentDbSizeKey, sizeB, nil)
}
