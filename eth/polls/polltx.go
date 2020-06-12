// +build evm

package polls

import (
	"github.com/gogo/protobuf/proto"
	"github.com/diademnetwork/go-diadem/plugin/types"
	"github.com/diademnetwork/diademchain"
	"github.com/diademnetwork/diademchain/receipts/common"
	"github.com/diademnetwork/diademchain/rpc/eth"
	"github.com/diademnetwork/diademchain/store"
	"github.com/pkg/errors"
)

type EthTxPoll struct {
	startBlock    uint64
	lastBlockRead uint64
}

func NewEthTxPoll(height uint64) *EthTxPoll {
	p := &EthTxPoll{
		startBlock:    height,
		lastBlockRead: height,
	}
	return p
}

func (p *EthTxPoll) Poll(blockStore store.BlockStore, state diademchain.ReadOnlyState, id string, _ diademchain.ReadReceiptHandler) (EthPoll, interface{}, error) {
	if p.lastBlockRead+1 > uint64(state.Block().Height) {
		return p, nil, nil
	}
	lastBlock, results, err := getTxHashes(state, p.lastBlockRead)
	if err != nil {
		return p, nil, nil
	}
	p.lastBlockRead = lastBlock
	return p, eth.EncBytesArray(results), nil
}

func (p *EthTxPoll) AllLogs(blockStore store.BlockStore, state diademchain.ReadOnlyState, id string, readReceipts diademchain.ReadReceiptHandler) (interface{}, error) {
	_, results, err := getTxHashes(state, p.startBlock)
	return eth.EncBytesArray(results), err
}

func getTxHashes(state diademchain.ReadOnlyState, lastBlockRead uint64) (uint64, [][]byte, error) {
	var txHashes [][]byte
	for height := lastBlockRead + 1; height < uint64(state.Block().Height); height++ {
		txHashList, err := common.GetTxHashList(state, height)
		if err != nil {
			return lastBlockRead, nil, errors.Wrapf(err, "reading tx hashes at height %d", height)
		}
		if len(txHashList) > 0 {
			txHashes = append(txHashes, txHashList...)
		}
		lastBlockRead = height
	}
	return lastBlockRead, txHashes, nil
}

func (p *EthTxPoll) LegacyPoll(blockStore store.BlockStore, state diademchain.ReadOnlyState, id string, _ diademchain.ReadReceiptHandler) (EthPoll, []byte, error) {
	if p.lastBlockRead+1 > uint64(state.Block().Height) {
		return p, nil, nil
	}

	var txHashes [][]byte
	for height := p.lastBlockRead + 1; height < uint64(state.Block().Height); height++ {
		txHashList, err := common.GetTxHashList(state, height)
		if err != nil {
			return p, nil, errors.Wrapf(err, "reading tx hash at heght %d", height)
		}
		if len(txHashList) > 0 {
			txHashes = append(txHashes, txHashList...)
		}
	}
	p.lastBlockRead = uint64(state.Block().Height)

	blocksMsg := types.EthFilterEnvelope_EthTxHashList{
		&types.EthTxHashList{EthTxHash: txHashes},
	}
	r, err := proto.Marshal(&types.EthFilterEnvelope{Message: &blocksMsg})
	return p, r, err
}
