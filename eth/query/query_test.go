// +build evm

package query

import (
	"bytes"
	"os"
	"testing"

	"github.com/diademnetwork/diademchain/events"
	"github.com/diademnetwork/diademchain/rpc/eth"
	"github.com/diademnetwork/diademchain/store"

	"github.com/diademnetwork/diademchain/receipts/common"
	"github.com/diademnetwork/diademchain/receipts/leveldb"

	"github.com/gogo/protobuf/proto"
	"github.com/diademnetwork/go-diadem"
	"github.com/diademnetwork/go-diadem/plugin/types"
	types1 "github.com/diademnetwork/go-diadem/types"
	"github.com/diademnetwork/diademchain"
	"github.com/diademnetwork/diademchain/eth/bdiadem"
	"github.com/diademnetwork/diademchain/eth/utils"
	"github.com/diademnetwork/diademchain/receipts/handler"
	"github.com/stretchr/testify/require"
)

const (
	allFilter = "{\"fromBlock\":\"earliest\",\"toBlock\":\"latest\",\"address\":\"\",\"topics\":[]}"
)

var (
	addr1 = diadem.MustParseAddress("chain:0xb16a379ec18d4093666f8f38b11a3071c920207d")
	addr2 = diadem.MustParseAddress("chain:0x5cecd1f7261e1f4c684e297be3edf03b825e01c4")
)

func TestQueryChain(t *testing.T) {
	testQueryChain(t, handler.ReceiptHandlerChain)
	os.RemoveAll(leveldb.Db_Filename)
	_, err := os.Stat(leveldb.Db_Filename)
	require.True(t, os.IsNotExist(err))
	testQueryChain(t, handler.ReceiptHandlerLevelDb)
}

func testQueryChain(t *testing.T, v handler.ReceiptHandlerVersion) {
	eventDispatcher := events.NewLogEventDispatcher()
	eventHandler := diademchain.NewDefaultEventHandler(eventDispatcher)
	receiptHandler, err := handler.NewReceiptHandler(v, eventHandler, handler.DefaultMaxReceipts)
	var writer diademchain.WriteReceiptHandler
	writer = receiptHandler

	require.NoError(t, err)
	state := common.MockState(0)

	state4 := common.MockStateAt(state, 4)
	mockEvent1 := []*types.EventData{
		{
			Topics:      []string{"topic1", "topic2", "topic3"},
			EncodedBody: []byte("somedata"),
			Address:     addr1.MarshalPB(),
		},
	}
	_, err = writer.CacheReceipt(state4, addr1, addr2, mockEvent1, nil)
	require.NoError(t, err)
	receiptHandler.CommitCurrentReceipt()

	protoBlock, err := GetPendingBlock(4, true, receiptHandler)
	require.NoError(t, err)
	blockInfo := types.EthBlockInfo{}
	require.NoError(t, proto.Unmarshal(protoBlock, &blockInfo))
	require.EqualValues(t, int64(4), blockInfo.Number)
	require.EqualValues(t, 1, len(blockInfo.Transactions))

	require.NoError(t, receiptHandler.CommitBlock(state4, 4))

	mockEvent2 := []*types.EventData{
		{
			Topics:      []string{"topic1"},
			EncodedBody: []byte("somedata"),
			Address:     addr1.MarshalPB(),
		},
	}
	state20 := common.MockStateAt(state, 20)
	_, err = writer.CacheReceipt(state20, addr1, addr2, mockEvent2, nil)
	require.NoError(t, err)
	receiptHandler.CommitCurrentReceipt()
	require.NoError(t, receiptHandler.CommitBlock(state20, 20))

	blockStore := store.NewMockBlockStore()

	state30 := common.MockStateAt(state, uint64(30))
	result, err := DeprecatedQueryChain(allFilter, blockStore, state30, receiptHandler)
	require.NoError(t, err, "error query chain, filter is %s", allFilter)
	var logs types.EthFilterLogList
	require.NoError(t, proto.Unmarshal(result, &logs), "unmarshalling EthFilterLogList")
	require.Equal(t, 2, len(logs.EthBlockLogs), "wrong number of logs returned")

	ethFilter, err := utils.UnmarshalEthFilter([]byte(allFilter))
	require.NoError(t, err)
	filterLogs, err := QueryChain(blockStore, state30, ethFilter, receiptHandler)
	require.NoError(t, err, "error query chain, filter is %s", ethFilter)
	require.Equal(t, 2, len(filterLogs), "wrong number of logs returned")

	require.NoError(t, receiptHandler.Close())
}

func TestMatchFilters(t *testing.T) {
	addr3 := &types1.Address{
		ChainId: "defult",
		Local:   []byte("test3333"),
	}
	addr4 := &types1.Address{
		ChainId: "defult",
		Local:   []byte("test4444"),
	}
	testEvents := []*diademchain.EventData{
		{
			Topics:  []string{"Topic1", "Topic2", "Topic3", "Topic4"},
			Address: addr3,
		},
		{
			Topics:  []string{"Topic5"},
			Address: addr3,
		},
	}
	testEventsG := []*types.EventData{
		{
			Topics:      []string{"Topic1", "Topic2", "Topic3", "Topic4"},
			Address:     addr3,
			EncodedBody: []byte("Some data"),
		},
		{
			Topics:  []string{"Topic5"},
			Address: addr3,
		},
	}
	ethFilter1 := eth.EthBlockFilter{
		Topics: [][]string{{"Topic1"}, nil, {"Topic3", "Topic4"}, {"Topic4"}},
	}
	ethFilter2 := eth.EthBlockFilter{
		Addresses: []diadem.LocalAddress{addr4.Local},
	}
	ethFilter3 := eth.EthBlockFilter{
		Topics: [][]string{{"Topic1"}},
	}
	ethFilter4 := eth.EthBlockFilter{
		Addresses: []diadem.LocalAddress{addr4.Local, addr3.Local},
		Topics:    [][]string{nil, nil, {"Topic2"}},
	}
	ethFilter5 := eth.EthBlockFilter{
		Topics: [][]string{{"Topic1"}, {"Topic6"}},
	}
	bdiademFilter := bdiadem.GenBdiademFilter(common.ConvertEventData(testEvents))

	require.True(t, MatchBdiademFilter(ethFilter1, bdiademFilter))
	require.False(t, MatchBdiademFilter(ethFilter2, bdiademFilter)) // address does not match
	require.True(t, MatchBdiademFilter(ethFilter3, bdiademFilter))  // one of the addresses mathch
	require.True(t, MatchBdiademFilter(ethFilter4, bdiademFilter))
	require.False(t, MatchBdiademFilter(ethFilter5, bdiademFilter))

	require.True(t, utils.MatchEthFilter(ethFilter1, *testEventsG[0]))
	require.False(t, utils.MatchEthFilter(ethFilter2, *testEventsG[0]))
	require.True(t, utils.MatchEthFilter(ethFilter3, *testEventsG[0]))
	require.False(t, utils.MatchEthFilter(ethFilter4, *testEventsG[0]))
	require.False(t, utils.MatchEthFilter(ethFilter5, *testEventsG[0]))

	require.False(t, utils.MatchEthFilter(ethFilter1, *testEventsG[1]))
	require.False(t, utils.MatchEthFilter(ethFilter2, *testEventsG[1]))
	require.False(t, utils.MatchEthFilter(ethFilter3, *testEventsG[1]))
	require.False(t, utils.MatchEthFilter(ethFilter4, *testEventsG[1]))
	require.False(t, utils.MatchEthFilter(ethFilter5, *testEventsG[1]))
}

func TestGetLogs(t *testing.T) {
	testGetLogs(t, handler.ReceiptHandlerChain)

	os.RemoveAll(leveldb.Db_Filename)
	_, err := os.Stat(leveldb.Db_Filename)
	require.True(t, os.IsNotExist(err))
	testGetLogs(t, handler.ReceiptHandlerLevelDb)
}

func testGetLogs(t *testing.T, v handler.ReceiptHandlerVersion) {
	os.RemoveAll(leveldb.Db_Filename)
	_, err := os.Stat(leveldb.Db_Filename)
	require.True(t, os.IsNotExist(err))

	eventDispatcher := events.NewLogEventDispatcher()
	eventHandler := diademchain.NewDefaultEventHandler(eventDispatcher)
	receiptHandler, err := handler.NewReceiptHandler(v, eventHandler, handler.DefaultMaxReceipts)
	var writer diademchain.WriteReceiptHandler
	writer = receiptHandler

	require.NoError(t, err)
	ethFilter := eth.EthBlockFilter{
		Topics: [][]string{{"Topic1"}, nil, {"Topic3", "Topic4"}, {"Topic4"}},
	}
	testEvents := []*types.EventData{
		{
			Topics:      []string{"Topic1", "Topic2", "Topic3", "Topic4"},
			Address:     addr1.MarshalPB(),
			EncodedBody: []byte("Some data"),
		},
		{
			Topics:  []string{"Topic5"},
			Address: addr1.MarshalPB(),
		},
	}

	testEventsG := []*types.EventData{
		{
			Topics:      []string{"Topic1", "Topic2", "Topic3", "Topic4"},
			Address:     addr1.MarshalPB(),
			EncodedBody: []byte("Some data"),
		},
		{
			Topics:  []string{"Topic5"},
			Address: addr1.MarshalPB(),
		},
	}
	state := common.MockState(1)
	state32 := common.MockStateAt(state, 32)
	txHash, err := writer.CacheReceipt(state32, addr1, addr2, testEventsG, nil)
	require.NoError(t, err)
	receiptHandler.CommitCurrentReceipt()
	require.NoError(t, receiptHandler.CommitBlock(state32, 32))

	state40 := common.MockStateAt(state, 40)
	txReceipt, err := receiptHandler.GetReceipt(state40, txHash)
	require.NoError(t, err)

	blockStore := store.NewMockBlockStore()

	logs, err := getTxHashLogs(blockStore, txReceipt, ethFilter, txHash)
	require.NoError(t, err, "getBlockLogs failed")
	require.Equal(t, len(logs), 1)
	require.Equal(t, logs[0].TransactionIndex, txReceipt.TransactionIndex)
	require.Equal(t, logs[0].TransactionHash, txReceipt.TxHash)
	require.True(t, 0 == bytes.Compare(logs[0].BlockHash, txReceipt.BlockHash))
	require.Equal(t, logs[0].BlockNumber, txReceipt.BlockNumber)
	require.True(t, 0 == bytes.Compare(logs[0].Address, txReceipt.CallerAddress.Local))
	require.True(t, 0 == bytes.Compare(logs[0].Data, testEvents[0].EncodedBody))
	require.Equal(t, len(logs[0].Topics), 4)
	require.True(t, 0 == bytes.Compare(logs[0].Topics[0], []byte(testEvents[0].Topics[0])))

	require.NoError(t, receiptHandler.Close())
}
