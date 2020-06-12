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
	return handler.ReceiptHandlerLegacyV2
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

func writeReceipt(
	state diademchain.State,
	caller, addr diadem.Address,
	events []*types.EventData,
	err error,
	eventHadler diademchain.EventHandler,
) (types.EvmTxReceipt, error) {
	var status int32
	if err == nil {
		status = 1
	} else {
		status = 0
	}
	block := state.Block()
	txReceipt := types.EvmTxReceipt{
		TransactionIndex:  state.Block().NumTxs,
		BlockHash:         block.GetLastBlockID().Hash,
		BlockNumber:       state.Block().Height,
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
			return types.EvmTxReceipt{}, errors.Wrap(errMarshal, "marhsal tx receipt")
		} else {
			return types.EvmTxReceipt{}, errors.Wrapf(err, "marshalling receipt err %v", errMarshal)
		}
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
		txReceipt.Logs = append(txReceipt.Logs, &pEvent)
	}

	return txReceipt, nil
}

func (r *ReceiptHandler) CacheReceipt(state diademchain.State, caller, addr diadem.Address, events []*types.EventData, err error) ([]byte, error) {
	txReceipt, errWrite := writeReceipt(state, caller, addr, events, err, r.eventHandler)
	if errWrite != nil {
		if err == nil {
			return nil, errors.Wrap(errWrite, "writing receipt")
		} else {
			return nil, errors.Wrapf(err, "error writing receipt %v", errWrite)
		}
	}
	postTxReceipt, errMarshal := proto.Marshal(&txReceipt)
	if errMarshal != nil {
		if err == nil {
			return nil, errors.Wrap(errMarshal, "marhsal tx receipt")
		} else {
			return nil, errors.Wrapf(err, "marshalling receipt err %v", errMarshal)
		}
	}
	height := common.BlockHeightToBytes(uint64(txReceipt.BlockNumber))
	bdiademState := store.PrefixKVStore(common.BdiademPrefix, state)
	bdiademState.Set(height, txReceipt.LogsBdiadem)
	txHashState := store.PrefixKVStore(common.TxHashPrefix, state)
	txHashState.Set(height, txReceipt.TxHash)

	receiptState := store.PrefixKVStore(common.ReceiptPrefix, state)
	receiptState.Set(txReceipt.TxHash, postTxReceipt)

	return txReceipt.TxHash, err
}

func (r *ReceiptHandler) SetFailStatusCurrentReceipt() {
}
