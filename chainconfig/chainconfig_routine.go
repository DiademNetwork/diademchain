package chainconfig

import (
	"runtime"
	"strconv"
	"time"

	godiadem "github.com/diademnetwork/go-diadem"
	"github.com/diademnetwork/go-diadem/auth"
	"github.com/diademnetwork/go-diadem/client"
	"github.com/diademnetwork/diademchain"
	"github.com/diademnetwork/diademchain/abci/backend"
	"github.com/diademnetwork/diademchain/config"
)

// ChainConfigRoutine periodically checks for pending features in the ChainConfig contract and
// automatically votes to enable those features.
type ChainConfigRoutine struct {
	cfg         *config.ChainConfigConfig
	chainID     string
	signer      auth.Signer
	address     godiadem.Address
	logger      *godiadem.Logger
	buildNumber uint64
	node        backend.Backend
}

// NewChainConfigRoutine returns a new instance of ChainConfigRoutine
func NewChainConfigRoutine(
	cfg *config.ChainConfigConfig,
	chainID string,
	nodeSigner auth.Signer,
	node backend.Backend,
	logger *godiadem.Logger,
) (*ChainConfigRoutine, error) {
	address := godiadem.Address{
		ChainID: chainID,
		Local:   godiadem.LocalAddressFromPublicKey(nodeSigner.PublicKey()),
	}
	build, err := strconv.ParseUint(diademchain.Build, 10, 64)
	if err != nil {
		build = 0
	}
	return &ChainConfigRoutine{
		cfg:         cfg,
		chainID:     chainID,
		signer:      nodeSigner,
		address:     address,
		logger:      logger,
		buildNumber: build,
		node:        node,
	}, nil
}

// RunWithRecovery should be run as a go-routine, it will auto-restart on panic unless it hits
// a runtime error.
func (cc *ChainConfigRoutine) RunWithRecovery() {
	defer func() {
		if r := recover(); r != nil {
			cc.logger.Error("recovered from panic in ChainConfigRoutine", "r", r)
			// Unless it's a runtime error restart the goroutine
			if _, ok := r.(runtime.Error); !ok {
				time.Sleep(30 * time.Second)
				cc.logger.Info("Restarting ChainConfigRoutine.")
				go cc.RunWithRecovery()
			}
		}
	}()

	// Give the node a bit of time to spin up
	if cc.cfg.EnableFeatureStartupDelay > 0 {
		time.Sleep(time.Duration(cc.cfg.EnableFeatureStartupDelay) * time.Second)
	}

	cc.run()
}

func (cc *ChainConfigRoutine) run() {
	for {
		if cc.node.IsValidator() {
			dappClient := client.NewDAppChainRPCClient(cc.chainID, cc.cfg.DAppChainWriteURI, cc.cfg.DAppChainReadURI)
			chainConfigClient, err := NewChainConfigClient(dappClient, cc.address, cc.signer, cc.logger)
			if err != nil {
				cc.logger.Error("Failed to create ChainConfigClient", "err", err)
			} else {
				// NOTE: errors are logged by the client, no need to log again
				_ = chainConfigClient.VoteToEnablePendingFeatures(cc.buildNumber)
			}
		}
		time.Sleep(time.Duration(cc.cfg.EnableFeatureInterval) * time.Second)
	}
}
