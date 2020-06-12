package common

import (
	"encoding/binary"
	"github.com/gogo/protobuf/proto"
	"github.com/diademnetwork/go-diadem/plugin/types"
	"github.com/diademnetwork/diademchain"
	"github.com/diademnetwork/diademchain/store"
	"github.com/pkg/errors"
)

const (
	StatusTxSuccess = int32(1)
	StatusTxFail    = int32(0)
)

var (
	ReceiptPrefix = []byte("receipt")
	BdiademPrefix   = []byte("bdiademFilter")
	TxHashPrefix  = []byte("txHash")
)

func GetTxHashList(state diademchain.ReadOnlyState, height uint64) ([][]byte, error) {
	receiptState := store.PrefixKVReader(TxHashPrefix, state)
	protHashList := receiptState.Get(BlockHeightToBytes(height))
	txHashList := types.EthTxHashList{}
	err := proto.Unmarshal(protHashList, &txHashList)
	return txHashList.EthTxHash, err
}

func AppendTxHashList(state diademchain.State, txHash [][]byte, height uint64) error {
	txHashList, err := GetTxHashList(state, height)
	if err != nil {
		return errors.Wrap(err, "getting tx hash list")
	}
	txHashList = append(txHashList, txHash...)

	postTxHashList, err := proto.Marshal(&types.EthTxHashList{EthTxHash: txHashList})
	if err != nil {
		return errors.Wrap(err, "marshal tx hash list")
	}
	txHashState := store.PrefixKVStore(TxHashPrefix, state)
	txHashState.Set(BlockHeightToBytes(height), postTxHashList)
	return nil
}

func GetBdiademFilter(state diademchain.ReadOnlyState, height uint64) []byte {
	bdiademState := store.PrefixKVReader(BdiademPrefix, state)
	return bdiademState.Get(BlockHeightToBytes(height))
}

func SetBdiademFilter(state diademchain.State, filter []byte, height uint64) {
	bdiademState := store.PrefixKVWriter(BdiademPrefix, state)
	bdiademState.Set(BlockHeightToBytes(height), filter)
}

func BlockHeightToBytes(height uint64) []byte {
	heightB := make([]byte, 8)
	binary.LittleEndian.PutUint64(heightB, height)
	return heightB
}

func ConvertEventData(events []*diademchain.EventData) []*types.EventData {

	typesEvents := make([]*types.EventData, 0, len(events))
	for _, event := range events {
		typeEvent := types.EventData(*event)
		typesEvents = append(typesEvents, &typeEvent)
	}
	return typesEvents
}
