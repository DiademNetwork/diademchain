// +build evm

package query

import (
	"fmt"

	"github.com/diademnetwork/diademchain/eth/bdiadem"
	"github.com/diademnetwork/diademchain/receipts/common"
	"github.com/diademnetwork/diademchain/rpc/eth"
	"github.com/diademnetwork/diademchain/store"
	"github.com/pkg/errors"

	"github.com/gogo/protobuf/proto"
	ptypes "github.com/diademnetwork/go-diadem/plugin/types"
	"github.com/diademnetwork/diademchain"
	"github.com/diademnetwork/diademchain/eth/utils"
	ctypes "github.com/tendermint/tendermint/rpc/core/types"
)

func QueryChain(
	blockStore store.BlockStore, state diademchain.ReadOnlyState, ethFilter eth.EthFilter,
	readReceipts diademchain.ReadReceiptHandler,
) ([]*ptypes.EthFilterLog, error) {
	start, err := eth.DecBlockHeight(state.Block().Height, eth.BlockHeight(ethFilter.FromBlock))
	if err != nil {
		return nil, err
	}
	end, err := eth.DecBlockHeight(state.Block().Height, eth.BlockHeight(ethFilter.ToBlock))
	if err != nil {
		return nil, err
	}

	return GetBlockLogRange(blockStore, state, start, end, ethFilter.EthBlockFilter, readReceipts)
}

func DeprecatedQueryChain(
	query string, blockStore store.BlockStore, state diademchain.ReadOnlyState,
	readReceipts diademchain.ReadReceiptHandler,
) ([]byte, error) {
	ethFilter, err := utils.UnmarshalEthFilter([]byte(query))
	if err != nil {
		return nil, err
	}
	start, err := utils.DeprecatedBlockNumber(string(ethFilter.FromBlock), uint64(state.Block().Height))
	if err != nil {
		return nil, err
	}
	end, err := utils.DeprecatedBlockNumber(string(ethFilter.ToBlock), uint64(state.Block().Height))
	if err != nil {
		return nil, err
	}

	eventLogs, err := GetBlockLogRange(blockStore, state, start, end, ethFilter.EthBlockFilter, readReceipts)
	if err != nil {
		return nil, err
	}

	return proto.Marshal(&ptypes.EthFilterLogList{EthBlockLogs: eventLogs})
}

func GetBlockLogRange(
	blockStore store.BlockStore,
	state diademchain.ReadOnlyState,
	from, to uint64,
	ethFilter eth.EthBlockFilter,
	readReceipts diademchain.ReadReceiptHandler,
) ([]*ptypes.EthFilterLog, error) {
	if from > to {
		return nil, fmt.Errorf("to block before end block")
	}
	eventLogs := []*ptypes.EthFilterLog{}

	for height := from; height <= to; height++ {
		blockLogs, err := GetBlockLogs(blockStore, state, ethFilter, height, readReceipts)
		if err != nil {
			return nil, err
		}
		eventLogs = append(eventLogs, blockLogs...)
	}
	return eventLogs, nil
}

func GetBlockLogs(
	blockStore store.BlockStore,
	state diademchain.ReadOnlyState,
	ethFilter eth.EthBlockFilter,
	height uint64,
	readReceipts diademchain.ReadReceiptHandler,
) ([]*ptypes.EthFilterLog, error) {
	bdiademFilter := common.GetBdiademFilter(state, height)
	if len(bdiademFilter) > 0 {
		if MatchBdiademFilter(ethFilter, bdiademFilter) {
			txHashList, err := common.GetTxHashList(state, height)
			if err != nil {
				return nil, errors.Wrapf(err, "txhash for block height %d", height)
			}
			var logsBlock []*ptypes.EthFilterLog
			for _, txHash := range txHashList {
				txReceipt, err := readReceipts.GetReceipt(state, txHash)
				if err != nil {
					return nil, errors.Wrap(err, "getting receipt")
				}
				logsTx, err := getTxHashLogs(blockStore, txReceipt, ethFilter, txHash)
				if err != nil {
					return nil, errors.Wrap(err, "logs for tx")
				}
				logsBlock = append(logsBlock, logsTx...)
			}
			return logsBlock, nil
		}
	}
	return nil, nil
}

func GetPendingBlockLogs(
	blockStore store.BlockStore, ethFilter eth.EthBlockFilter, receiptHandler diademchain.ReadReceiptHandler,
) ([]*ptypes.EthFilterLog, error) {
	txHashList := receiptHandler.GetPendingTxHashList()
	var logsBlock []*ptypes.EthFilterLog
	for _, txHash := range txHashList {
		txReceipt, err := receiptHandler.GetPendingReceipt(txHash)
		if err != nil {
			return nil, errors.Wrap(err, "cannot find pending tx receipt matching hash")
		}
		logsTx, err := getTxHashLogs(blockStore, txReceipt, ethFilter, txHash)
		if err != nil {
			return nil, errors.Wrap(err, "logs for tx")
		}
		logsBlock = append(logsBlock, logsTx...)
	}
	return logsBlock, nil
}

func getTxHashLogs(blockStore store.BlockStore, txReceipt ptypes.EvmTxReceipt, filter eth.EthBlockFilter, txHash []byte) ([]*ptypes.EthFilterLog, error) {
	var blockLogs []*ptypes.EthFilterLog

	// Timestamp added here rather than being stored in the event itself so
	// as to avoid altering the data saved to the app-store.
	var timestamp int64
	if len(txReceipt.Logs) > 0 {
		height := int64(txReceipt.BlockNumber)
		var blockResult *ctypes.ResultBlock
		blockResult, err := blockStore.GetBlockByHeight(&height)
		if err != nil {
			return blockLogs, errors.Wrapf(err, "getting block info for height %v", height)
		}
		timestamp = blockResult.Block.Header.Time.Unix()
	}

	for i, eventLog := range txReceipt.Logs {
		if utils.MatchEthFilter(filter, *eventLog) {
			var topics [][]byte
			for _, topic := range eventLog.Topics {
				topics = append(topics, []byte(topic))
			}
			blockLogs = append(blockLogs, &ptypes.EthFilterLog{
				Removed:          false,
				LogIndex:         int64(i),
				TransactionIndex: txReceipt.TransactionIndex,
				TransactionHash:  txHash,
				BlockHash:        txReceipt.BlockHash,
				BlockNumber:      txReceipt.BlockNumber,
				Address:          eventLog.Address.Local,
				Data:             eventLog.EncodedBody,
				Topics:           topics,
				BlockTime:        timestamp,
			})
		}
	}
	return blockLogs, nil
}

func MatchBdiademFilter(ethFilter eth.EthBlockFilter, bdiademFilter []byte) bool {
	bFilter := bdiadem.NewBdiademFilter()
	if len(ethFilter.Addresses) > 0 {
		found := false
		for _, addr := range ethFilter.Addresses {
			if bFilter.Contains(bdiademFilter, []byte(addr)) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	for _, topics := range ethFilter.Topics {
		if len(topics) > 0 {
			found := false
			for _, topic := range topics {
				if bFilter.Contains(bdiademFilter, []byte(topic)) {
					found = true
					break
				}
			}
			if !found {
				return false
			}
		}
	}
	return true
}
