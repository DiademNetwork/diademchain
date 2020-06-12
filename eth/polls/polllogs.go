// +build evm

package polls

import (
	"fmt"

	"github.com/pkg/errors"

	"github.com/diademnetwork/diademchain/store"

	"github.com/gogo/protobuf/proto"
	"github.com/diademnetwork/go-diadem/plugin/types"
	"github.com/diademnetwork/diademchain"
	"github.com/diademnetwork/diademchain/eth/query"
	"github.com/diademnetwork/diademchain/eth/utils"
	"github.com/diademnetwork/diademchain/rpc/eth"
)

type EthLogPoll struct {
	filter        eth.EthFilter
	lastBlockRead uint64
}

func NewEthLogPoll(filter string) (*EthLogPoll, error) {
	ethFilter, err := utils.UnmarshalEthFilter([]byte(filter))
	if err != nil {
		return nil, err
	}
	p := &EthLogPoll{
		filter:        ethFilter,
		lastBlockRead: uint64(0),
	}
	return p, nil
}

func (p *EthLogPoll) Poll(blockStore store.BlockStore, state diademchain.ReadOnlyState, id string, readReceipts diademchain.ReadReceiptHandler) (EthPoll, interface{}, error) {
	start, err := eth.DecBlockHeight(state.Block().Height, p.filter.FromBlock)
	if err != nil {
		return p, nil, err
	}
	end, err := eth.DecBlockHeight(state.Block().Height, p.filter.ToBlock)
	if err != nil {
		return p, nil, err
	}

	if start > end {
		return p, nil, errors.New("Filter FromBlock is greater than ToBlock")
	}

	if start <= p.lastBlockRead {
		start = p.lastBlockRead + 1
		if start > end {
			return p, nil, nil
		}
	}

	eventLogs, err := query.GetBlockLogRange(blockStore, state, start, end, p.filter.EthBlockFilter, readReceipts)
	if err != nil {
		return p, nil, err
	}
	newLogPoll := &EthLogPoll{
		filter:        p.filter,
		lastBlockRead: end,
	}
	return newLogPoll, eth.EncLogs(eventLogs), nil
}

func (p *EthLogPoll) AllLogs(blockStore store.BlockStore, state diademchain.ReadOnlyState, id string, readReceipts diademchain.ReadReceiptHandler) (interface{}, error) {
	start, err := eth.DecBlockHeight(state.Block().Height, p.filter.FromBlock)
	if err != nil {
		return nil, err
	}
	end, err := eth.DecBlockHeight(state.Block().Height, p.filter.ToBlock)
	if err != nil {
		return nil, err
	}
	if start > end {
		return nil, errors.New("Filter FromBlock is greater than ToBlock")
	}

	eventLogs, err := query.GetBlockLogRange(blockStore, state, start, end, p.filter.EthBlockFilter, readReceipts)
	if err != nil {
		return nil, err
	}
	return eth.EncLogs(eventLogs), nil
}

func (p *EthLogPoll) LegacyPoll(blockStore store.BlockStore, state diademchain.ReadOnlyState, id string, readReceipts diademchain.ReadReceiptHandler) (EthPoll, []byte, error) {
	start, err := eth.DecBlockHeight(state.Block().Height, p.filter.FromBlock)
	if err != nil {
		return p, nil, err
	}
	end, err := eth.DecBlockHeight(state.Block().Height, p.filter.ToBlock)
	if err != nil {
		return p, nil, err
	}

	if start <= p.lastBlockRead {
		start = p.lastBlockRead + 1
		if start > end {
			return p, nil, fmt.Errorf("filter start after filter end")
		}
	}

	eventLogs, err := query.GetBlockLogRange(blockStore, state, start, end, p.filter.EthBlockFilter, readReceipts)
	if err != nil {
		return p, nil, err
	}
	newLogPoll := &EthLogPoll{
		filter:        p.filter,
		lastBlockRead: end,
	}

	blocksMsg := types.EthFilterEnvelope_EthFilterLogList{
		&types.EthFilterLogList{EthBlockLogs: eventLogs},
	}
	r, err := proto.Marshal(&types.EthFilterEnvelope{Message: &blocksMsg})
	return newLogPoll, r, err
}
