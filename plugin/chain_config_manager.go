package plugin

import (
	"strconv"

	"github.com/diademnetwork/go-diadem"
	contract "github.com/diademnetwork/go-diadem/plugin/contractpb"
	"github.com/diademnetwork/diademchain"
	"github.com/diademnetwork/diademchain/builtin/plugins/chainconfig"
	regcommon "github.com/diademnetwork/diademchain/registry"
	"github.com/pkg/errors"
)

var (
	// ErrChainConfigContractNotFound indicates that the ChainConfig contract hasn't been deployed yet.
	ErrChainConfigContractNotFound = errors.New("[ChainConfigManager] ChainContract contract not found")
)

// ChainConfigManager implements diademchain.ChainConfigManager interface
type ChainConfigManager struct {
	ctx   contract.Context
	state diademchain.State
	build uint64
}

// NewChainConfigManager attempts to create an instance of ChainConfigManager.
func NewChainConfigManager(pvm *PluginVM, state diademchain.State) (*ChainConfigManager, error) {
	caller := diadem.RootAddress(pvm.State.Block().ChainID)
	contractAddr, err := pvm.Registry.Resolve("chainconfig")
	if err != nil {
		if err == regcommon.ErrNotFound {
			return nil, ErrChainConfigContractNotFound
		}
		return nil, err
	}
	readOnly := false
	ctx := contract.WrapPluginContext(pvm.CreateContractContext(caller, contractAddr, readOnly))
	build, err := strconv.ParseUint(diademchain.Build, 10, 64)
	if err != nil {
		build = 0
	}
	return &ChainConfigManager{
		ctx:   ctx,
		state: state,
		build: build,
	}, nil
}

func (c *ChainConfigManager) EnableFeatures(blockHeight int64) error {
	features, err := chainconfig.EnableFeatures(c.ctx, uint64(blockHeight), c.build)
	if err != nil {
		// When an unsupported feature has been activated by the rest of the chain
		// panic to prevent the node from processing any further blocks until it's
		// upgraded to a new build that supports the feature.
		if err == chainconfig.ErrFeatureNotSupported {
			panic(err)
		}
		return err
	}
	for _, feature := range features {
		c.state.SetFeature(feature.Name, true)
	}
	return nil
}
