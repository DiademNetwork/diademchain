// +build !evm

package diademchain

import (
	"github.com/diademnetwork/go-diadem"
	"github.com/diademnetwork/go-diadem/plugin/types"
)

type WriteReceiptHandler interface {
	CacheReceipt(state State, caller, addr diadem.Address, events []*types.EventData, err error) ([]byte, error)
}
