package main

import (
	"strings"
	"testing"

	proto "github.com/gogo/protobuf/proto"
	diadem "github.com/diademnetwork/go-diadem"
	lauth "github.com/diademnetwork/go-diadem/auth"
	"github.com/diademnetwork/go-diadem/types"
	"github.com/diademnetwork/diademchain"
	"github.com/diademnetwork/diademchain/auth"
	registry "github.com/diademnetwork/diademchain/registry/factory"
	"github.com/diademnetwork/diademchain/store"
	"github.com/diademnetwork/diademchain/vm"
	"github.com/stretchr/testify/require"
	abci "github.com/tendermint/tendermint/abci/types"
	"golang.org/x/crypto/ed25519"
)

// Tx handlers must not process txs in which the caller doesn't match the signer.
func TestTxHandlerWithInvalidCaller(t *testing.T) {
	_, alicePrivKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)

	bobPubKey, _, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)

	createRegistry, err := registry.NewRegistryFactory(registry.LatestRegistryVersion)
	require.NoError(t, err)

	vmManager := vm.NewManager()
	router := diademchain.NewTxRouter()
	router.HandleDeliverTx(1, diademchain.GeneratePassthroughRouteHandler(&vm.DeployTxHandler{Manager: vmManager, CreateRegistry: createRegistry}))
	router.HandleDeliverTx(2, diademchain.GeneratePassthroughRouteHandler(&vm.CallTxHandler{Manager: vmManager}))

	txMiddleWare := []diademchain.TxMiddleware{
		auth.SignatureTxMiddleware,
		auth.NonceTxMiddleware,
	}

	state := diademchain.NewStoreState(nil, store.NewMemStore(), abci.Header{ChainID: "default"}, nil, nil)
	rootHandler := diademchain.MiddlewareTxHandler(txMiddleWare, router, nil)
	signer := lauth.NewEd25519Signer(alicePrivKey)
	caller := diadem.Address{
		ChainID: "default",
		Local:   diadem.LocalAddressFromPublicKey(bobPubKey),
	}

	// Try to process txs in which Alice attempts to impersonate Bob
	_, err = rootHandler.ProcessTx(state, createTxWithInvalidCaller(t, signer, caller, &vm.DeployTx{
		VmType: vm.VMType_PLUGIN,
		Code:   nil,
		Name:   "hello",
	}, 1, 1), false)
	require.Error(t, err)
	require.True(t, strings.HasPrefix(err.Error(), "Origin doesn't match caller"))

	_, err = rootHandler.ProcessTx(state, createTxWithInvalidCaller(t, signer, caller, &vm.CallTx{
		VmType: vm.VMType_PLUGIN,
	}, 2, 2), false)
	require.Error(t, err)
	require.True(t, strings.HasPrefix(err.Error(), "Origin doesn't match caller"))
}

func createTxWithInvalidCaller(t *testing.T, signer lauth.Signer, caller diadem.Address,
	tx proto.Message, txType uint32, nonce uint64) []byte {
	payload, err := proto.Marshal(tx)
	require.NoError(t, err)

	msgBytes, err := proto.Marshal(&vm.MessageTx{
		From: caller.MarshalPB(),
		To:   diadem.RootAddress("default").MarshalPB(),
		Data: payload,
	})
	require.NoError(t, err)

	txBytes, err := proto.Marshal(&types.Transaction{
		Id:   txType,
		Data: msgBytes,
	})
	require.NoError(t, err)

	nonceTxBytes, err := proto.Marshal(&auth.NonceTx{
		Inner:    txBytes,
		Sequence: nonce,
	})
	require.NoError(t, err)

	signedTx := lauth.SignTx(signer, nonceTxBytes)
	signedTxBytes, err := proto.Marshal(signedTx)
	require.NoError(t, err)
	return signedTxBytes
}
