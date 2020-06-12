// +build !evm

package polls

import (
	"github.com/diademnetwork/diademchain"
	"github.com/diademnetwork/diademchain/rpc/eth"
	"github.com/diademnetwork/diademchain/store"
)

type EthSubscriptions struct {
}

func (s EthSubscriptions) LegacyAddLogPoll(_ string, _ uint64) (string, error) {
	return "", nil
}

func (s EthSubscriptions) AddBlockPoll(_ uint64) string {
	return ""
}

func (s EthSubscriptions) AddTxPoll(_ uint64) string {
	return ""
}

func (s *EthSubscriptions) LegacyPoll(_ store.BlockStore, _ diademchain.ReadOnlyState, _ string, _ diademchain.ReadReceiptHandler) ([]byte, error) {
	return nil, nil
}

func (s *EthSubscriptions) Remove(_ string) {
}

func (s EthSubscriptions) Poll(_ store.BlockStore, _ diademchain.ReadOnlyState, _ string, _ diademchain.ReadReceiptHandler) (interface{}, error) {
	return nil, nil
}

func (s EthSubscriptions) AllLogs(_ store.BlockStore, _ diademchain.ReadOnlyState, _ string, _ diademchain.ReadReceiptHandler) (interface{}, error) {
	return nil, nil
}

func (s EthSubscriptions) AddLogPoll(_ eth.EthFilter, _ uint64) (string, error) {
	return "", nil
}

func NewEthSubscriptions() *EthSubscriptions {
	return &EthSubscriptions{}
}
