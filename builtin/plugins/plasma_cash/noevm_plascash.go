// +build !evm

package plasma_cash

import (
	pctypes "github.com/diademnetwork/go-diadem/builtin/types/plasma_cash"
	"github.com/diademnetwork/go-diadem/plugin"
	contract "github.com/diademnetwork/go-diadem/plugin/contractpb"
)

// Dummy file so you can build the server without the EVM

type PlasmaCash struct {
}

func (c *PlasmaCash) Meta() (plugin.Meta, error) {
	return plugin.Meta{
		Name:    "plasmacash",
		Version: "1.0.0",
	}, nil
}

func (c *PlasmaCash) Init(ctx contract.Context, req pctypes.PlasmaCashInitRequest) error {
	return nil
}

func (c *PlasmaCash) PlasmaTxRequest(ctx contract.Context, req *pctypes.PlasmaTxRequest) error {
	return nil
}

var Contract plugin.Contract = contract.MakePluginContract(&PlasmaCash{})
