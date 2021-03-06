// +build evm

package query

import (
	"bytes"
	"fmt"

	"github.com/gogo/protobuf/proto"
	"github.com/diademnetwork/go-diadem/plugin/types"
	"github.com/diademnetwork/diademchain"
	"github.com/diademnetwork/diademchain/eth/utils"
	"github.com/diademnetwork/diademchain/receipts/common"
	"github.com/diademnetwork/diademchain/rpc/eth"
	"github.com/diademnetwork/diademchain/store"
	"github.com/pkg/errors"
	ctypes "github.com/tendermint/tendermint/rpc/core/types"
)

var (
	searchBlockSize = uint64(20)
)

func GetBlockByNumber(
	blockStore store.BlockStore, state diademchain.ReadOnlyState, height int64, full bool,
	readReceipts diademchain.ReadReceiptHandler,
) (resp eth.JsonBlockObject, err error) {
	// todo make information about pending block available
	if height > state.Block().Height {
		return resp, errors.New("get block information for pending blocks not implemented yet")
	}

	var blockResult *ctypes.ResultBlock
	blockResult, err = blockStore.GetBlockByHeight(&height)
	if err != nil {
		return resp, err
	}

	var proposalAddress eth.Data

	if blockResult.Block.Header.ProposerAddress != nil {
		proposalAddress = eth.EncBytes(blockResult.Block.Header.ProposerAddress)
	} else {
		proposalAddress = eth.ZeroedData20Bytes
	}

	blockInfo := eth.JsonBlockObject{
		ParentHash:       eth.EncBytes(blockResult.Block.Header.LastBlockID.Hash),
		Timestamp:        eth.EncInt(int64(blockResult.Block.Header.Time.Unix())),
		GasLimit:         eth.EncInt(0),
		GasUsed:          eth.EncInt(0),
		Size:             eth.EncInt(0),
		Transactions:     nil,
		Nonce:            eth.ZeroedData8Bytes,
		Sha3Uncles:       eth.ZeroedData32Bytes,
		TransactionsRoot: eth.ZeroedData32Bytes,
		StateRoot:        eth.ZeroedData32Bytes,
		ReceiptsRoot:     eth.ZeroedData32Bytes,
		Miner:            proposalAddress,
		Difficulty:       eth.ZeroedQuantity,
		TotalDifficulty:  eth.ZeroedQuantity,
		ExtraData:        eth.ZeroedData,
		Uncles:           []eth.Data{},
	}

	// These three fields are null for pending blocks.
	blockInfo.Hash = eth.EncBytes(blockResult.BlockMeta.BlockID.Hash)
	blockInfo.Number = eth.EncInt(height)
	blockInfo.LogsBdiadem = eth.EncBytes(common.GetBdiademFilter(state, uint64(height)))

	txHashList, err := common.GetTxHashList(state, uint64(height))
	if err != nil {
		return resp, errors.Wrapf(err, "get tx hash list at height %v", height)
	}
	for _, hash := range txHashList {
		if full {
			txObj, err := GetTxByHash(state, hash, readReceipts)
			if err != nil {
				return resp, errors.Wrapf(err, "txObj for hash %v", hash)
			}
			blockInfo.Transactions = append(blockInfo.Transactions, txObj)
		} else {
			blockInfo.Transactions = append(blockInfo.Transactions, eth.EncBytes(hash))
		}
	}

	if len(blockInfo.Transactions) == 0 {
		blockInfo.Transactions = make([]interface{}, 0)
	}

	return blockInfo, nil
}

func GetNumEvmTxBlock(blockStore store.BlockStore, state diademchain.ReadOnlyState, height int64) (uint64, error) {
	// todo make information about pending block available.
	// Should be able to get transaction count from receipt object.
	if height > state.Block().Height {
		return 0, errors.New("get number of transactions for pending blocks, not implemented yet")
	}

	var blockResults *ctypes.ResultBlockResults
	blockResults, err := blockStore.GetBlockResults(&height)
	if err != nil {
		return 0, errors.Wrapf(err, "results for block %v", height)
	}

	numEvmTx := uint64(0)
	for _, deliverTx := range blockResults.Results.DeliverTx {
		if deliverTx.Info == utils.DeployEvm || deliverTx.Info == utils.CallEVM {
			numEvmTx++
		}
	}
	return numEvmTx, nil
}

// todo find better method of doing this. Maybe use a blockhash index.
func GetBlockHeightFromHash(blockStore store.BlockStore, state diademchain.ReadOnlyState, hash []byte) (int64, error) {
	start := uint64(state.Block().Height)
	var end uint64
	if uint64(start) > searchBlockSize {
		end = uint64(start) - searchBlockSize
	} else {
		end = 1
	}

	for start > 0 {
		var info *ctypes.ResultBlockchainInfo
		info, err := blockStore.GetBlockRangeByHeight(int64(end), int64(start))
		if err != nil {
			return 0, err
		}

		if err != nil {
			return 0, err
		}
		for i := int(len(info.BlockMetas) - 1); i >= 0; i-- {
			if 0 == bytes.Compare(hash, info.BlockMetas[i].BlockID.Hash) {
				return info.BlockMetas[i].Header.Height, nil //    int64(int(end) + i), nil
			}
		}

		if end == 1 {
			return 0, fmt.Errorf("can't find block to match hash")
		}

		start = end
		if uint64(start) > searchBlockSize {
			end = uint64(start) - searchBlockSize
		} else {
			end = 1
		}
	}
	return 0, fmt.Errorf("can't find block to match hash")
}

func DeprecatedGetBlockByNumber(
	blockStore store.BlockStore, state diademchain.ReadOnlyState, height int64, full bool,
	readReceipts diademchain.ReadReceiptHandler,
) ([]byte, error) {
	var blockresult *ctypes.ResultBlock
	iHeight := height
	blockresult, err := blockStore.GetBlockByHeight(&iHeight)
	if err != nil {
		return nil, err
	}
	blockinfo := types.EthBlockInfo{
		Hash:       blockresult.BlockMeta.BlockID.Hash,
		ParentHash: blockresult.Block.Header.LastBlockID.Hash,

		Timestamp: int64(blockresult.Block.Header.Time.Unix()),
	}
	if state.Block().Height == height {
		blockinfo.Number = 0
	} else {
		blockinfo.Number = height
	}

	bdiademFilter := common.GetBdiademFilter(state, uint64(height))
	blockinfo.LogsBdiadem = bdiademFilter

	txHashList, err := common.GetTxHashList(state, uint64(height))
	if err != nil {
		return nil, errors.Wrap(err, "getting tx hash")
	}
	if full {
		for _, txHash := range txHashList {
			txObj, err := DeprecatedGetTxByHash(state, txHash, readReceipts)
			if err != nil {
				return nil, errors.Wrap(err, "marshall tx object")
			}
			blockinfo.Transactions = append(blockinfo.Transactions, txObj)
		}
	} else {
		blockinfo.Transactions = txHashList
	}

	return proto.Marshal(&blockinfo)
}

func GetPendingBlock(height int64, full bool, readReceipts diademchain.ReadReceiptHandler) ([]byte, error) {
	blockinfo := types.EthBlockInfo{
		Number: int64(height),
	}
	txHashList := readReceipts.GetPendingTxHashList()
	if full {
		for _, txHash := range txHashList {
			txReceipt, err := readReceipts.GetPendingReceipt(txHash)
			if err != nil {
				return nil, errors.Wrap(err, "reading receipt")
			}
			txReceiptProto, err := proto.Marshal(&txReceipt)
			if err != nil {
				return nil, errors.Wrap(err, "marshall receipt")
			}
			blockinfo.Transactions = append(blockinfo.Transactions, txReceiptProto)
		}
	} else {
		blockinfo.Transactions = txHashList
	}

	return proto.Marshal(&blockinfo)
}

func DeprecatedGetBlockByHash(
	blockStore store.BlockStore, state diademchain.ReadOnlyState, hash []byte, full bool,
	readReceipts diademchain.ReadReceiptHandler,
) ([]byte, error) {
	start := uint64(state.Block().Height)
	var end uint64
	if uint64(start) > searchBlockSize {
		end = uint64(start) - searchBlockSize
	} else {
		end = 1
	}

	for start > 0 {
		var info *ctypes.ResultBlockchainInfo
		info, err := blockStore.GetBlockRangeByHeight(int64(end), int64(start))
		if err != nil {
			return nil, err
		}

		if err != nil {
			return nil, err
		}
		for i := int(len(info.BlockMetas) - 1); i >= 0; i-- {
			if 0 == bytes.Compare(hash, info.BlockMetas[i].BlockID.Hash) {
				return DeprecatedGetBlockByNumber(blockStore, state, info.BlockMetas[i].Header.Height, full, readReceipts)
			}
		}

		if end == 1 {
			return nil, fmt.Errorf("can't find block to match hash")
		}

		start = end
		if uint64(start) > searchBlockSize {
			end = uint64(start) - searchBlockSize
		} else {
			end = 1
		}
	}
	return nil, fmt.Errorf("can't find block to match hash")
}
