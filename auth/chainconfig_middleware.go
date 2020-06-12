package auth

import (
	"fmt"

	diadem "github.com/diademnetwork/go-diadem"
	"github.com/diademnetwork/go-diadem/plugin/contractpb"
	"github.com/diademnetwork/diademchain"
)

// NewChainConfigMiddleware returns middleware that verifies signed txs using either
// SignedTxMiddleware or MultiChainSignatureTxMiddleware, it switches the underlying middleware
// based on the on-chain and off-chain auth config settings.
func NewChainConfigMiddleware(
	authConfig *Config,
	createAddressMapperCtx func(state diademchain.State) (contractpb.Context, error),
) diademchain.TxMiddlewareFunc {
	return diademchain.TxMiddlewareFunc(func(
		state diademchain.State,
		txBytes []byte,
		next diademchain.TxHandlerFunc,
		isCheckTx bool,
	) (diademchain.TxHandlerResult, error) {
		chains := getEnabledChains(authConfig.Chains, state)
		if len(chains) > 0 {
			mw := NewMultiChainSignatureTxMiddleware(chains, createAddressMapperCtx)
			return mw(state, txBytes, next, isCheckTx)
		}

		return SignatureTxMiddleware(state, txBytes, next, isCheckTx)
	})
}

// Filters out any auth.ChainConfig(s) that haven't been enabled by the majority of validators.
func getEnabledChains(chains map[string]ChainConfig, state diademchain.State) map[string]ChainConfig {
	enabledChains := map[string]ChainConfig{}
	for chainID, config := range chains {
		if state.FeatureEnabled(diademchain.AuthSigTxFeaturePrefix+chainID, false) {
			enabledChains[chainID] = config
		}
	}

	// Ensure the chain will always process native ed25519 txs by default.
	if len(enabledChains) > 0 {
		curChainID := state.Block().ChainID
		if _, found := enabledChains[curChainID]; !found {
			enabledChains[curChainID] = ChainConfig{
				TxType:      DiademSignedTxType,
				AccountType: NativeAccountType,
			}
		}
	}
	return enabledChains
}

// ResolveAccountAddress takes a local or foreign account address and returns the address used
// to identify the account on this chain.
func ResolveAccountAddress(
	account diadem.Address, state diademchain.State, authCfg *Config,
	createAddressMapperCtx func(state diademchain.State) (contractpb.Context, error),
) (diadem.Address, error) {
	chains := getEnabledChains(authCfg.Chains, state)
	if len(chains) > 0 {
		chain, found := chains[account.ChainID]
		if !found {
			return diadem.Address{}, fmt.Errorf("unknown chain ID %s", account.ChainID)
		}

		switch chain.AccountType {
		case NativeAccountType:
			return account, nil

		case MappedAccountType:
			addr, err := getMappedAccountAddress(state, account, createAddressMapperCtx)
			if err != nil {
				return diadem.Address{}, err
			}
			return addr, nil

		default:
			return diadem.Address{},
				fmt.Errorf("invalid account type %v for chain ID %s", chain.AccountType, account.ChainID)
		}
	}

	if account.ChainID != state.Block().ChainID {
		return diadem.Address{}, fmt.Errorf("unknown chain ID %s", account.ChainID)
	}
	return account, nil
}
