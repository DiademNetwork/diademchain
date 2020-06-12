package chain

import (
	"crypto/sha256"

	"github.com/gogo/protobuf/proto"
	"github.com/diademnetwork/go-diadem"
	"github.com/diademnetwork/go-diadem/plugin/types"
	"github.com/diademnetwork/diademchain"
	"github.com/diademnetwork/diademchain/eth/bdiadem"
	"github.com/diademnetwork/diademchain/receipts/common"
	"github.com/diademnetwork/diademchain/receipts/handler"
	"github.com/diademnetwork/diademchain/store"
	"github.com/pkg/errors"
)

// ReceiptHandler implements diademchain.ReadReceiptHandler, diademchain.WriteReceiptHandler, and
// diademchain.ReceiptHandlerStore interfaces in diadem builds prior to 495.
type ReceiptHandler struct {
	eventHandler diademchain.EventHandler
}

func NewReceiptHandler(eventHandler diademchain.EventHandler) *ReceiptHandler {
	return &ReceiptHandler{
		eventHandler: eventHandler,
	}
}

func (r *ReceiptHandler) Version() handler.ReceiptHandlerVersion {
	return handler.ReceiptHandlerLegacyV1
}

func (r *ReceiptHandler) GetReceipt(state diademchain.ReadOnlyState, txHash []byte) (types.EvmTxReceipt, error) {
	receiptState := store.PrefixKVReader(common.ReceiptPrefix, state)
	txReceiptProto := receiptState.Get(txHash)
	txReceipt := types.EvmTxReceipt{}
	if txReceiptProto == nil {
		return txReceipt, errors.New("Tx receipt not found")
	}
	err := proto.Unmarshal(txReceiptProto, &txReceipt)
	return txReceipt, err

}

func (r *ReceiptHandler) GetPendingReceipt(txHash []byte) (types.EvmTxReceipt, error) {
	return types.EvmTxReceipt{}, errors.New("pending receipt not found")
}

func (r *ReceiptHandler) GetCurrentReceipt() *types.EvmTxReceipt {
	return nil
}

func (r *ReceiptHandler) GetPendingTxHashList() [][]byte {
	return nil
}

func (r *ReceiptHandler) Close() error {
	return nil
}

func (r *ReceiptHandler) ClearData() error {
	return nil
}

func (r *ReceiptHandler) CommitCurrentReceipt() {
}

func (r *ReceiptHandler) DiscardCurrentReceipt() {
}

func (r *ReceiptHandler) CommitBlock(state diademchain.State, height int64) error {
	return nil
}

func (r *ReceiptHandler) CacheReceipt(state diademchain.State, caller, addr diadem.Address, events []*types.EventData, err error) ([]byte, error) {
	block := state.Block()
	var status int32
	if err == nil {
		status = 1
	} else {
		status = 0
	}
	txReceipt := types.EvmTxReceipt{
		TransactionIndex:  block.NumTxs,
		BlockHash:         block.GetLastBlockID().Hash,
		BlockNumber:       block.Height,
		CumulativeGasUsed: 0,
		GasUsed:           0,
		ContractAddress:   addr.Local,
		LogsBdiadem:         bdiadem.GenBdiademFilter(events),
		Status:            status,
		CallerAddress:     caller.MarshalPB(),
	}

	preTxReceipt, errMarshal := proto.Marshal(&txReceipt)
	if errMarshal != nil {
		if err == nil {
			return []byte{}, errMarshal
		} else {
			return []byte{}, err
		}
	}
	h := sha256.New()
	h.Write(preTxReceipt)
	txHash := h.Sum(nil)

	txReceipt.TxHash = txHash
	blockHeight := uint64(txReceipt.BlockNumber)
	for _, event := range events {
		event.TxHash = txHash
		_ = r.eventHandler.Post(blockHeight, event)
		txReceipt.Logs = append(txReceipt.Logs, event)
	}

	postTxReceipt, errMarshal := proto.Marshal(&txReceipt)
	if errMarshal != nil {
		if err == nil {
			return []byte{}, errMarshal
		} else {
			return []byte{}, err
		}
	}

	receiptState := store.PrefixKVStore(common.ReceiptPrefix, state)
	receiptState.Set(txHash, postTxReceipt)

	height := common.BlockHeightToBytes(blockHeight)
	bdiademState := store.PrefixKVStore(common.BdiademPrefix, state)
	bdiademState.Set(height, txReceipt.LogsBdiadem)
	txHashState := store.PrefixKVStore(common.TxHashPrefix, state)
	txHashState.Set(height, txReceipt.TxHash)

	return txHash, err
}

func (r *ReceiptHandler) SetFailStatusCurrentReceipt() {
}
