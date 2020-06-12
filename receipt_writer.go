// +build evm

package diademchain

import (
	eth_types "github.com/ethereum/go-ethereum/core/types"
	"github.com/diademnetwork/go-diadem"
	"github.com/diademnetwork/go-diadem/plugin/types"
)

type WriteReceiptHandler interface {
	GetEventsFromLogs(logs []*eth_types.Log, blockHeight int64, caller, contract diadem.Address, input []byte) []*types.EventData
	CacheReceipt(state State, caller, addr diadem.Address, events []*types.EventData, err error) ([]byte, error)
}
