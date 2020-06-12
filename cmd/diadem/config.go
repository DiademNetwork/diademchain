package main

import (
	"bytes"
	"encoding/json"

	"github.com/gogo/protobuf/proto"
	"github.com/diademnetwork/go-diadem"
	cctypes "github.com/diademnetwork/go-diadem/builtin/types/chainconfig"
	dwtypes "github.com/diademnetwork/go-diadem/builtin/types/deployer_whitelist"
	ktypes "github.com/diademnetwork/go-diadem/builtin/types/karma"
	"github.com/diademnetwork/go-diadem/plugin/contractpb"
	"github.com/diademnetwork/go-diadem/types"
	"github.com/diademnetwork/diademchain/builtin/plugins/chainconfig"
	"github.com/diademnetwork/diademchain/builtin/plugins/dpos"
	"github.com/diademnetwork/diademchain/builtin/plugins/dposv2"
	"github.com/diademnetwork/diademchain/builtin/plugins/dposv3"
	"github.com/diademnetwork/diademchain/builtin/plugins/karma"
	"github.com/diademnetwork/diademchain/config"
	"github.com/diademnetwork/diademchain/plugin"
)

func marshalInit(pb proto.Message) (json.RawMessage, error) {
	var buf bytes.Buffer
	marshaler, err := contractpb.MarshalerFactory(plugin.EncodingType_JSON)
	if err != nil {
		return nil, err
	}
	err = marshaler.Marshal(&buf, pb)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(buf.Bytes()), nil
}

func defaultGenesis(cfg *config.Config, validator *diadem.Validator) (*config.Genesis, error) {
	contracts := []config.ContractConfig{
		{
			VMTypeName: "plugin",
			Format:     "plugin",
			Name:       "coin",
			Location:   "coin:1.0.0",
		},
	}

	if cfg.DPOSVersion == 1 {
		dposInit, err := marshalInit(&dpos.InitRequest{
			Params: &dpos.Params{
				WitnessCount:        21,
				ElectionCycleLength: 604800, // one week
				MinPowerFraction:    5,      // 20%
			},
			Validators: []*diadem.Validator{
				validator,
			},
		})
		if err != nil {
			return nil, err
		}

		contracts = append(contracts, config.ContractConfig{
			VMTypeName: "plugin",
			Format:     "plugin",
			Name:       "dpos",
			Location:   "dpos:1.0.0",
			Init:       dposInit,
		})
	} else if cfg.DPOSVersion == 2 {
		dposV2Init, err := marshalInit(&dposv2.InitRequest{
			Params: &dposv2.Params{
				ValidatorCount:      21,
				ElectionCycleLength: 604800, // one week
			},
			Validators: []*diadem.Validator{
				validator,
			},
		})
		if err != nil {
			return nil, err
		}

		contracts = append(contracts, config.ContractConfig{
			VMTypeName: "plugin",
			Format:     "plugin",
			Name:       "dposV2",
			Location:   "dposV2:2.0.0",
			Init:       dposV2Init,
		})
	} else if cfg.DPOSVersion == 3 {
		dposV3Init, err := marshalInit(&dposv3.InitRequest{
			Params: &dposv3.Params{
				ValidatorCount: 21,
			},
			Validators: []*diadem.Validator{
				validator,
			},
		})
		if err != nil {
			return nil, err
		}

		contracts = append(contracts, config.ContractConfig{
			VMTypeName: "plugin",
			Format:     "plugin",
			Name:       "dposV3",
			Location:   "dposV3:3.0.0",
			Init:       dposV3Init,
		})
	}

	//If this is enabled lets default to giving a genesis file with the plasma_cash contract
	if cfg.PlasmaCash.ContractEnabled {
		contracts = append(contracts,
			config.ContractConfig{
				VMTypeName: "plugin",
				Format:     "plugin",
				Name:       "plasmacash",
				Location:   "plasmacash:1.0.0",
			})
	}

	if cfg.TransferGateway.ContractEnabled {
		contracts = append(contracts,
			config.ContractConfig{
				VMTypeName: "plugin",
				Format:     "plugin",
				Name:       "ethcoin",
				Location:   "ethcoin:1.0.0",
			})
	}

	if cfg.AddressMapperContractEnabled() {
		contracts = append(contracts,
			config.ContractConfig{
				VMTypeName: "plugin",
				Format:     "plugin",
				Name:       "addressmapper",
				Location:   "addressmapper:0.1.0",
			})
	}

	if cfg.TransferGateway.ContractEnabled {
		contracts = append(contracts,
			config.ContractConfig{
				VMTypeName: "plugin",
				Format:     "plugin",
				Name:       "gateway",
				Location:   "gateway:0.1.0",
			})
	}

	if cfg.DiademCoinTransferGateway.ContractEnabled {
		contracts = append(contracts,
			config.ContractConfig{
				VMTypeName: "plugin",
				Format:     "plugin",
				Name:       "diademcoin-gateway",
				Location:   "diademcoin-gateway:0.1.0",
			})
	}

	if cfg.ChainConfig.ContractEnabled {

		ownerAddr := diadem.LocalAddressFromPublicKey(validator.PubKey)
		contractOwner := &types.Address{
			ChainId: "default",
			Local:   ownerAddr,
		}
		chainConfigInitRequest := cctypes.InitRequest{
			Owner: contractOwner,
			Params: &cctypes.Params{
				VoteThreshold:         67,
				NumBlockConfirmations: 10,
			},
			Features: []*cctypes.Feature{
				&cctypes.Feature{
					Name:   "test",
					Status: chainconfig.FeatureWaiting,
				},
			},
		}

		chainConfigInit, err := marshalInit(&chainConfigInitRequest)
		if err != nil {
			return nil, err
		}

		contracts = append(contracts, config.ContractConfig{
			VMTypeName: "plugin",
			Format:     "plugin",
			Name:       "chainconfig",
			Location:   "chainconfig:1.0.0",
			Init:       chainConfigInit,
		})
	}

	if cfg.DeployerWhitelist.ContractEnabled {

		ownerAddr := diadem.LocalAddressFromPublicKey(validator.PubKey)
		contractOwner := &types.Address{
			ChainId: "default",
			Local:   ownerAddr,
		}
		dwInitRequest := dwtypes.InitRequest{
			Owner: contractOwner,
		}

		dwInit, err := marshalInit(&dwInitRequest)
		if err != nil {
			return nil, err
		}

		contracts = append(contracts, config.ContractConfig{
			VMTypeName: "plugin",
			Format:     "plugin",
			Name:       "deployerwhitelist",
			Location:   "deployerwhitelist:1.0.0",
			Init:       dwInit,
		})
	}

	if cfg.Karma.Enabled {
		karmaInitRequest := ktypes.KarmaInitRequest{
			Sources: []*ktypes.KarmaSourceReward{
				{Name: "example-award-token", Reward: 1, Target: ktypes.KarmaSourceTarget_DEPLOY},
			},
			Upkeep: &ktypes.KarmaUpkeepParams{
				Cost:   1,
				Period: 3600,
			},
			Config: &ktypes.KarmaConfig{MinKarmaToDeploy: karma.DefaultUpkeepCost},
		}
		oracle, err := diadem.ParseAddress(cfg.Oracle)
		if err == nil {
			karmaInitRequest.Oracle = oracle.MarshalPB()
		}
		karmaInit, err := marshalInit(&karmaInitRequest)

		if err != nil {
			return nil, err
		}
		contracts = append(contracts, config.ContractConfig{
			VMTypeName: "plugin",
			Format:     "plugin",
			Name:       "karma",
			Location:   "karma:1.0.0",
			Init:       karmaInit,
		})
	}

	return &config.Genesis{
		Contracts: contracts,
	}, nil
}
