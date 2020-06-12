package chainconfig

import (
	"strings"

	"github.com/diademnetwork/go-diadem"
	godiadem "github.com/diademnetwork/go-diadem"
	"github.com/diademnetwork/go-diadem/auth"
	cctypes "github.com/diademnetwork/go-diadem/builtin/types/chainconfig"
	"github.com/diademnetwork/go-diadem/client"
	"github.com/pkg/errors"
)

type (
	ListFeaturesRequest   = cctypes.ListFeaturesRequest
	ListFeaturesResponse  = cctypes.ListFeaturesResponse
	Feature               = cctypes.Feature
	EnableFeatureRequest  = cctypes.EnableFeatureRequest
	EnableFeatureResponse = cctypes.EnableFeatureResponse
)

const (
	// FeaturePending status indicates a feature hasn't been enabled by majority of validators yet.
	FeaturePending = cctypes.Feature_PENDING
	// FeatureWaiting status indicates a feature has been enabled by majority of validators, but
	// hasn't been activated yet because not enough blocks confirmations have occurred yet.
	FeatureWaiting = cctypes.Feature_WAITING
	// FeatureEnabled status indicates a feature has been enabled by majority of validators, and
	// has been activated on the chain.
	FeatureEnabled = cctypes.Feature_ENABLED
	// FeatureDisabled is not currently used.
	FeatureDisabled = cctypes.Feature_DISABLED
)

// ChainConfigClient is used to enable pending features in the ChainConfig contract.
type ChainConfigClient struct {
	Address  godiadem.Address
	contract *client.Contract
	caller   godiadem.Address
	logger   *godiadem.Logger
	signer   auth.Signer
}

// NewChainConfigClient returns ChainConfigClient instance
func NewChainConfigClient(
	diademClient *client.DAppChainRPCClient,
	caller godiadem.Address,
	signer auth.Signer,
	logger *godiadem.Logger,
) (*ChainConfigClient, error) {
	chainConfigAddr, err := diademClient.Resolve("chainconfig")
	if err != nil {
		return nil, errors.Wrap(err, "failed to resolve ChainConfig contract address")
	}
	return &ChainConfigClient{
		Address:  chainConfigAddr,
		contract: client.NewContract(diademClient, chainConfigAddr.Local),
		caller:   caller,
		signer:   signer,
		logger:   logger,
	}, nil
}

// VoteToEnablePendingFeatures is called periodically by ChainConfigRoutine
// to enable pending feature if it's supported in this current build
func (cc *ChainConfigClient) VoteToEnablePendingFeatures(buildNumber uint64) error {
	var resp ListFeaturesResponse
	if _, err := cc.contract.StaticCall(
		"ListFeatures",
		&ListFeaturesRequest{},
		cc.caller,
		&resp,
	); err != nil {
		cc.logger.Error("Failed to retrieve features from ChainConfig contract", "err", err)
		return err
	}

	features := resp.Features
	featureNames := make([]string, 0)
	for _, feature := range features {
		if feature.Status == FeaturePending &&
			feature.BuildNumber <= buildNumber &&
			feature.AutoEnable &&
			!cc.hasVoted(feature) {
			featureNames = append(featureNames, feature.Name)
		}
	}

	if len(featureNames) > 0 {
		var resp EnableFeatureResponse
		if _, err := cc.contract.Call(
			"EnableFeature",
			&EnableFeatureRequest{Names: featureNames},
			cc.signer,
			&resp,
		); err != nil {
			cc.logger.Error(
				"Encountered an error while trying to auto-enable features",
				"features", strings.Join(featureNames, ","), "err", err,
			)
			return err
		}
		cc.logger.Info("Auto-enabled features", "features", strings.Join(featureNames, ","))
	}
	return nil
}

// Check if this validator has already voted to enable this feature
func (cc *ChainConfigClient) hasVoted(feature *Feature) bool {
	for _, v := range feature.Validators {
		validator := diadem.UnmarshalAddressPB(v)
		if validator.Compare(cc.caller) == 0 {
			return true
		}
	}
	return false
}
