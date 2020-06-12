package common

import (
	godiademplugin "github.com/diademnetwork/go-diadem/plugin"
	"github.com/diademnetwork/diademchain/builtin/plugins/address_mapper"
	"github.com/diademnetwork/diademchain/builtin/plugins/chainconfig"
	"github.com/diademnetwork/diademchain/builtin/plugins/deployer_whitelist"
	"github.com/diademnetwork/diademchain/builtin/plugins/dpos"
	"github.com/diademnetwork/diademchain/builtin/plugins/dposv2"
	"github.com/diademnetwork/diademchain/builtin/plugins/dposv3"
	"github.com/diademnetwork/diademchain/builtin/plugins/ethcoin"
	"github.com/diademnetwork/diademchain/builtin/plugins/gateway"
	"github.com/diademnetwork/diademchain/builtin/plugins/karma"
	"github.com/diademnetwork/diademchain/builtin/plugins/plasma_cash"
	"github.com/diademnetwork/diademchain/cmd/diadem/replay"
	"github.com/diademnetwork/diademchain/config"
	"github.com/diademnetwork/diademchain/plugin"
)

func NewDefaultContractsLoader(cfg *config.Config) plugin.Loader {
	contracts := []godiademplugin.Contract{}
	//For a quick way for other chains to just build new contracts into diadem, like gamechain
	contracts = append(contracts, builtinContracts...)

	if cfg.DPOSVersion == 3 {
		contracts = append(contracts, dposv3.Contract)
	} else if cfg.DPOSVersion == 2 {
		//We need to load both dposv3 and dposv2 for migration
		contracts = append(contracts, dposv2.Contract, dposv3.Contract)
	}

	if cfg.DPOSVersion == 1 || cfg.BootLegacyDPoS {
		//Plasmachain or old legacy chain need dposv1 to be able to bootstrap the chain.
		contracts = append(contracts, dpos.Contract)
	}
	if cfg.PlasmaCash.ContractEnabled {
		contracts = append(contracts, plasma_cash.Contract)
	}
	if cfg.Karma.Enabled || cfg.Karma.ContractEnabled {
		contracts = append(contracts, karma.Contract)
	}
	if cfg.TransferGateway.ContractEnabled {
		contracts = append(contracts, ethcoin.Contract)
	}
	if cfg.ChainConfig.ContractEnabled {
		contracts = append(contracts, chainconfig.Contract)
	}
	if cfg.DeployerWhitelist.ContractEnabled {
		contracts = append(contracts, deployer_whitelist.Contract)
	}

	if cfg.AddressMapperContractEnabled() {
		contracts = append(contracts, address_mapper.Contract)
	}

	if cfg.TransferGateway.ContractEnabled {
		if cfg.TransferGateway.Unsafe {
			contracts = append(contracts, gateway.UnsafeContract)
		} else {
			contracts = append(contracts, gateway.Contract)
		}
	}

	if cfg.DiademCoinTransferGateway.ContractEnabled {
		if cfg.DiademCoinTransferGateway.Unsafe {
			contracts = append(contracts, gateway.UnsafeDiademCoinContract)
		} else {
			contracts = append(contracts, gateway.DiademCoinContract)
		}
	}

	loader := plugin.NewStaticLoader(contracts...)
	loader.SetContractOverrides(replay.ContractOverrides())
	return loader
}
