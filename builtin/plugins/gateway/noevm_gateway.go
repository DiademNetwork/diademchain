// +build !evm

package gateway

import (
	tgtypes "github.com/diademnetwork/go-diadem/builtin/types/transfer_gateway"
	"github.com/diademnetwork/go-diadem/plugin"
	contract "github.com/diademnetwork/go-diadem/plugin/contractpb"
)

type (
	InitRequest = tgtypes.TransferGatewayInitRequest
)

type Gateway struct {
	diademCoinTG bool
}

type UnsafeGateway struct {
	Gateway
}

func (gw *Gateway) Meta() (plugin.Meta, error) {
	if gw.diademCoinTG {
		return plugin.Meta{
			Name:    "diademcoin-gateway",
			Version: "0.1.0",
		}, nil
	} else {
		return plugin.Meta{
			Name:    "gateway",
			Version: "0.1.0",
		}, nil
	}
}

func (gw *Gateway) Init(ctx contract.Context, req *InitRequest) error {
	return nil
}

var Contract plugin.Contract = contract.MakePluginContract(&Gateway{})
var UnsafeContract plugin.Contract = contract.MakePluginContract(&UnsafeGateway{Gateway{}})

var DiademCoinContract plugin.Contract = contract.MakePluginContract(&Gateway{})
var UnsafeDiademCoinContract plugin.Contract = contract.MakePluginContract(&UnsafeGateway{Gateway{}})
