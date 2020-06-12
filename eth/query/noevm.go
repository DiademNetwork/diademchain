// +build !evm

package query

import (
	"github.com/diademnetwork/go-diadem/plugin/types"
	"github.com/diademnetwork/diademchain"
	"github.com/diademnetwork/diademchain/rpc/eth"
	"github.com/diademnetwork/diademchain/store"
)

func DeprecatedQueryChain(_ string, _ store.BlockStore, _ diademchain.ReadOnlyState, _ diademchain.ReadReceiptHandler) ([]byte, error) {
	return nil, nil
}

func GetBlockByNumber(
	_ store.BlockStore, _ diademchain.ReadOnlyState, _ int64, _ bool, _ diademchain.ReadReceiptHandler,
) (eth.JsonBlockObject, error) {
	return eth.JsonBlockObject{}, nil
}

func DeprecatedGetBlockByNumber(
	_ store.BlockStore, _ diademchain.ReadOnlyState, _ int64, _ bool, _ diademchain.ReadReceiptHandler,
) ([]byte, error) {
	return nil, nil
}

func GetPendingBlock(_ int64, _ bool, _ diademchain.ReadReceiptHandler) ([]byte, error) {
	return nil, nil
}

func DeprecatedGetBlockByHash(
	_ store.BlockStore, _ diademchain.ReadOnlyState, _ []byte, _ bool, _ diademchain.ReadReceiptHandler,
) ([]byte, error) {
	return nil, nil
}

func DeprecatedGetTxByHash(_ diademchain.ReadOnlyState, _ []byte, _ diademchain.ReadReceiptHandler) ([]byte, error) {
	return nil, nil
}

func GetBlockHeightFromHash(_ store.BlockStore, _ diademchain.ReadOnlyState, _ []byte) (int64, error) {
	return 0, nil
}

func GetNumEvmTxBlock(_ store.BlockStore, _ diademchain.ReadOnlyState, _ int64) (uint64, error) {
	return 0, nil
}

func GetTxByHash(_ diademchain.ReadOnlyState, _ []byte, _ diademchain.ReadReceiptHandler) (eth.JsonTxObject, error) {
	return eth.JsonTxObject{}, nil
}

func GetTxByBlockAndIndex(_ store.BlockStore, _ diademchain.ReadOnlyState, _, _ uint64, _ diademchain.ReadReceiptHandler) (txObj eth.JsonTxObject, err error) {
	return eth.JsonTxObject{}, nil
}

func QueryChain(
	_ store.BlockStore, _ diademchain.ReadOnlyState, _ eth.EthFilter, _ diademchain.ReadReceiptHandler,
) ([]*types.EthFilterLog, error) {
	return nil, nil
}
