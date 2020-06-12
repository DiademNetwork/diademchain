// +build evm

package auth

import (
	"context"
	"testing"

	"github.com/gogo/protobuf/proto"
	"github.com/diademnetwork/go-diadem/auth"
	godiademplugin "github.com/diademnetwork/go-diadem/plugin"
	"github.com/diademnetwork/go-diadem/plugin/contractpb"
	"github.com/stretchr/testify/require"
	abci "github.com/tendermint/tendermint/abci/types"
	"golang.org/x/crypto/ed25519"

	"github.com/diademnetwork/diademchain"
	"github.com/diademnetwork/diademchain/builtin/plugins/address_mapper"
	"github.com/diademnetwork/diademchain/store"
)

func TestChainConfigMiddlewareSingleChain(t *testing.T) {
	origBytes := []byte("hello")
	_, privKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		panic(err)
	}
	signer := auth.NewEd25519Signer([]byte(privKey))
	signedTx := auth.SignTx(signer, origBytes)
	signedTxBytes, err := proto.Marshal(signedTx)
	require.NoError(t, err)
	state := diademchain.NewStoreState(nil, store.NewMemStore(), abci.Header{ChainID: "default"}, nil, nil)
	fakeCtx := godiademplugin.CreateFakeContext(addr1, addr1)
	addresMapperAddr := fakeCtx.CreateContract(address_mapper.Contract)
	amCtx := contractpb.WrapPluginContext(fakeCtx.WithAddress(addresMapperAddr))
	authConfig := Config{
		Chains: map[string]ChainConfig{},
	}

	chainConfigMiddleware := NewChainConfigMiddleware(&authConfig, func(state diademchain.State) (contractpb.Context, error) { return amCtx, nil })
	_, err = chainConfigMiddleware.ProcessTx(state, signedTxBytes,
		func(state diademchain.State, txBytes []byte, isCheckTx bool) (diademchain.TxHandlerResult, error) {
			require.Equal(t, txBytes, origBytes)
			return diademchain.TxHandlerResult{}, nil
		}, false,
	)
	require.NoError(t, err)
}

func TestChainConfigMiddlewareMultipleChain(t *testing.T) {
	state := diademchain.NewStoreState(nil, store.NewMemStore(), abci.Header{ChainID: defaultDiademChainId}, nil, nil)
	state.SetFeature(diademchain.AuthSigTxFeaturePrefix+"default", true)
	state.SetFeature(diademchain.AuthSigTxFeaturePrefix+"tron", true)
	state.SetFeature(diademchain.AuthSigTxFeaturePrefix+"eth", true)
	fakeCtx := godiademplugin.CreateFakeContext(addr1, addr1)
	addresMapperAddr := fakeCtx.CreateContract(address_mapper.Contract)
	amCtx := contractpb.WrapPluginContext(fakeCtx.WithAddress(addresMapperAddr))

	ctx := context.WithValue(state.Context(), ContextKeyOrigin, origin)

	chains := map[string]ChainConfig{
		"default": {
			TxType:      DiademSignedTxType,
			AccountType: NativeAccountType,
		},
		"eth": {
			TxType:      EthereumSignedTxType,
			AccountType: MappedAccountType,
		},
		"tron": {
			TxType:      TronSignedTxType,
			AccountType: MappedAccountType,
		},
	}
	authConfig := Config{
		Chains: chains,
	}

	tmx := NewChainConfigMiddleware(
		&authConfig,
		func(state diademchain.State) (contractpb.Context, error) { return amCtx, nil },
	)

	txSigned := mockEd25519SignedTx(t, priKey1)
	_, err := throttleMiddlewareHandler(tmx, state, txSigned, ctx)
	require.NoError(t, err)
}
