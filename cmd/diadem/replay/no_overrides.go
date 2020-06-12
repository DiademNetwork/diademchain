// +build !plasmachain

package replay

import (
	"github.com/diademnetwork/diademchain/config"
	"github.com/diademnetwork/diademchain/plugin"
)

func ContractOverrides() plugin.ContractOverrideMap {
	return nil
}

func OverrideConfig(cfg *config.Config, blockHeight int64) *config.Config {
	return cfg
}
