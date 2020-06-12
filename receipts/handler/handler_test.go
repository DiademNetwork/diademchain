package handler

import (
	"bytes"
	"os"
	"testing"

	"github.com/diademnetwork/go-diadem"
	"github.com/diademnetwork/go-diadem/plugin/types"
	"github.com/diademnetwork/go-diadem/util"
	"github.com/diademnetwork/diademchain"
	"github.com/diademnetwork/diademchain/eth/utils"
	"github.com/diademnetwork/diademchain/receipts/common"
	"github.com/diademnetwork/diademchain/receipts/leveldb"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
	abci "github.com/tendermint/tendermint/abci/types"
)

var (
	addr1 = diadem.MustParseAddress("chain:0xb16a379ec18d4093666f8f38b11a3071c920207d")
	addr2 = diadem.MustParseAddress("chain:0x5cecd1f7261e1f4c684e297be3edf03b825e01c4")
)

func TestReceiptsHandlerChain(t *testing.T) {
	testHandlerDepreciated(t, ReceiptHandlerChain)

	os.RemoveAll(leveldb.Db_Filename)
	_, err := os.Stat(leveldb.Db_Filename)
	require.True(t, os.IsNotExist(err))
	testHandler(t, ReceiptHandlerLevelDb)
}

func testHandlerDepreciated(t *testing.T, v ReceiptHandlerVersion) {
	height := uint64(1)
	state := common.MockState(height)

	handler, err := NewReceiptHandler(v, &diademchain.DefaultEventHandler{}, DefaultMaxReceipts)
	require.NoError(t, err)

	writer := handler
	receiptHandler := handler

	var txHashList [][]byte
	for txNum := 0; txNum < 20; txNum++ {
		if txNum%2 == 0 {
			stateI := common.MockStateTx(state, height, uint64(txNum))
			_, err = writer.CacheReceipt(stateI, addr1, addr2, []*types.EventData{}, nil)
			require.NoError(t, err)
			txHash, err := writer.CacheReceipt(stateI, addr1, addr2, []*types.EventData{}, nil)
			require.NoError(t, err)

			if txNum == 10 {
				receiptHandler.SetFailStatusCurrentReceipt()
			}
			receiptHandler.CommitCurrentReceipt()
			txHashList = append(txHashList, txHash)
		}
	}

	require.EqualValues(t, int(10), len(handler.receiptsCache))
	require.EqualValues(t, int(10), len(txHashList))

	var reader diademchain.ReadReceiptHandler
	reader = handler

	pendingHashList := reader.GetPendingTxHashList()
	require.EqualValues(t, 10, len(pendingHashList))

	for index, hash := range pendingHashList {
		receipt, err := reader.GetPendingReceipt(hash)
		require.NoError(t, err)
		require.EqualValues(t, 0, bytes.Compare(hash, receipt.TxHash))
		require.EqualValues(t, index*2, receipt.TransactionIndex)
		if index == 5 {
			require.EqualValues(t, common.StatusTxFail, receipt.Status)
		} else {
			require.EqualValues(t, common.StatusTxSuccess, receipt.Status)
		}
	}

	err = receiptHandler.CommitBlock(state, int64(height))
	require.NoError(t, err)

	pendingHashList = reader.GetPendingTxHashList()
	require.EqualValues(t, 0, len(pendingHashList))

	for index, txHash := range txHashList {
		txReceipt, err := reader.GetReceipt(state, txHash)
		require.NoError(t, err)
		require.EqualValues(t, 0, bytes.Compare(txHash, txReceipt.TxHash))
		require.EqualValues(t, index*2, txReceipt.TransactionIndex)
		if index == 5 {
			require.EqualValues(t, common.StatusTxFail, txReceipt.Status)
		} else {
			require.EqualValues(t, common.StatusTxSuccess, txReceipt.Status)
		}
	}

	require.NoError(t, receiptHandler.Close())
	require.NoError(t, receiptHandler.ClearData())
}

func testHandler(t *testing.T, v ReceiptHandlerVersion) {
	height := uint64(1)
	state := common.MockState(height)

	handler, err := NewReceiptHandler(v, &diademchain.DefaultEventHandler{}, DefaultMaxReceipts)
	require.NoError(t, err)

	var writer diademchain.WriteReceiptHandler
	writer = handler

	var receiptHandler diademchain.ReceiptHandlerStore
	receiptHandler = handler

	var txHashList [][]byte

	// mock block
	for nonce := 0; nonce < 20; nonce++ {
		var txError error
		var resp abci.ResponseDeliverTx
		diademchain.NewSequence(util.PrefixKey([]byte("nonce"), addr1.Bytes())).Next(state)
		var txHash []byte

		if nonce%2 == 0 { // mock EVM transaction
			stateI := common.MockStateTx(state, height, uint64(nonce))
			_, err = writer.CacheReceipt(stateI, addr1, addr2, []*types.EventData{}, nil)
			require.NoError(t, err)
			txHash, err = writer.CacheReceipt(stateI, addr1, addr2, []*types.EventData{}, nil)
			require.NoError(t, err)
			if nonce == 18 { // mock error
				receiptHandler.SetFailStatusCurrentReceipt()
				txError = errors.New("Some EVM error")
			}
			if nonce == 0 { // mock deploy transaction
				resp.Data = []byte("proto with contract address and tx hash")
				resp.Info = utils.DeployEvm
			} else { // mock call transaction
				resp.Data = txHash
				resp.Info = utils.CallEVM
			}
		} else { // mock non-EVM transaction
			resp.Data = []byte("Go transaction results")
			resp.Info = utils.CallPlugin
		}

		// mock Application.processTx
		if txError != nil {
			receiptHandler.DiscardCurrentReceipt()
		} else {
			if resp.Info == utils.CallEVM || resp.Info == utils.DeployEvm {
				receiptHandler.CommitCurrentReceipt()
				txHashList = append(txHashList, txHash)
			}
		}
	}

	require.EqualValues(t, int(9), len(handler.receiptsCache))
	require.EqualValues(t, int(9), len(txHashList))

	var reader diademchain.ReadReceiptHandler
	reader = handler

	pendingHashList := reader.GetPendingTxHashList()
	require.EqualValues(t, 9, len(pendingHashList))

	for index, hash := range pendingHashList {
		receipt, err := reader.GetPendingReceipt(hash)
		require.NoError(t, err)
		require.EqualValues(t, 0, bytes.Compare(hash, receipt.TxHash))
		require.EqualValues(t, int64(index*2+1), receipt.Nonce)
		require.EqualValues(t, index, receipt.TransactionIndex)
		require.EqualValues(t, common.StatusTxSuccess, receipt.Status)
	}

	err = receiptHandler.CommitBlock(state, int64(height))
	require.NoError(t, err)

	pendingHashList = reader.GetPendingTxHashList()
	require.EqualValues(t, 0, len(pendingHashList))

	for index, txHash := range txHashList {
		txReceipt, err := reader.GetReceipt(state, txHash)
		require.NoError(t, err)
		require.EqualValues(t, 0, bytes.Compare(txHash, txReceipt.TxHash))
		require.EqualValues(t, index*2+1, txReceipt.Nonce)
		require.EqualValues(t, index, txReceipt.TransactionIndex)
		require.EqualValues(t, common.StatusTxSuccess, txReceipt.Status)
	}

	require.NoError(t, receiptHandler.Close())
	require.NoError(t, receiptHandler.ClearData())
}
