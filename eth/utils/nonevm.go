// +build !evm

package utils

import (
	ptypes "github.com/diademnetwork/go-diadem/plugin/types"
	"github.com/diademnetwork/diademchain/rpc/eth"
)

func GetId() string {
	return ""
}

func UnmarshalEthFilter(_ []byte) (eth.EthFilter, error) {
	return eth.EthFilter{}, nil
}

func MatchEthFilter(_ eth.EthBlockFilter, _ ptypes.EventData) bool {
	return true
}
