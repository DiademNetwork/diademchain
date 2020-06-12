package auth

import (
	"context"
	"testing"

	proto "github.com/gogo/protobuf/proto"
	"github.com/stretchr/testify/require"
	abci "github.com/tendermint/tendermint/abci/types"
	"golang.org/x/crypto/ed25519"

	diadem "github.com/diademnetwork/go-diadem"
	"github.com/diademnetwork/go-diadem/auth"
	"github.com/diademnetwork/diademchain"
	"github.com/diademnetwork/diademchain/store"
)

func TestSignatureTxMiddleware(t *testing.T) {
	origBytes := []byte("hello")
	_, privKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		panic(err)
	}
	signer := auth.NewEd25519Signer([]byte(privKey))
	signedTx := auth.SignTx(signer, origBytes)
	signedTxBytes, err := proto.Marshal(signedTx)
	require.NoError(t, err)
	state := diademchain.NewStoreState(nil, store.NewMemStore(), abci.Header{}, nil, nil)
	SignatureTxMiddleware.ProcessTx(state, signedTxBytes,
		func(state diademchain.State, txBytes []byte, isCheckTx bool) (diademchain.TxHandlerResult, error) {
			require.Equal(t, txBytes, origBytes)
			return diademchain.TxHandlerResult{}, nil
		}, false,
	)
}
func TestSignatureTxMiddlewareMultipleTxSameBlock(t *testing.T) {
	pubkey, _, err := ed25519.GenerateKey(nil)
	if err != nil {
		panic(err)
	}

	nonceTxBytes, err := proto.Marshal(&NonceTx{
		Inner:    []byte{},
		Sequence: 1,
	})
	require.NoError(t, err)

	nonceTxBytes2, err := proto.Marshal(&NonceTx{
		Inner:    []byte{},
		Sequence: 2,
	})
	require.NoError(t, err)

	origin := diadem.Address{
		ChainID: "default",
		Local:   diadem.LocalAddressFromPublicKey(pubkey),
	}

	ctx := context.WithValue(context.Background(), ContextKeyOrigin, origin)
	ctx = context.WithValue(ctx, ContextKeyCheckTx, true)

	state := diademchain.NewStoreState(ctx, store.NewMemStore(), abci.Header{Height: 27}, nil, nil)

	_, err = NonceTxMiddleware(state, nonceTxBytes,
		func(state diademchain.State, txBytes []byte, isCheckTx bool) (diademchain.TxHandlerResult, error) {
			return diademchain.TxHandlerResult{}, nil
		}, false,
	)
	require.Nil(t, err)
	NonceTxPostNonceMiddleware(state, nonceTxBytes, diademchain.TxHandlerResult{}, nil)

	//State is reset on every run
	ctx2 := context.WithValue(context.Background(), ContextKeyOrigin, origin)
	state2 := diademchain.NewStoreState(ctx2, store.NewMemStore(), abci.Header{Height: 27}, nil, nil)
	ctx2 = context.WithValue(ctx2, ContextKeyCheckTx, true)

	//If we get the same sequence number in same block we should get an error
	_, err = NonceTxMiddleware(state2, nonceTxBytes,
		func(state2 diademchain.State, txBytes []byte, isCheckTx bool) (diademchain.TxHandlerResult, error) {
			return diademchain.TxHandlerResult{}, nil
		}, true,
	)
	require.Errorf(t, err, "sequence number does not match")
	//	NonceTxPostNonceMiddleware shouldnt get called on an error

	//State is reset on every run
	ctx3 := context.WithValue(context.Background(), ContextKeyOrigin, origin)
	state3 := diademchain.NewStoreState(ctx3, store.NewMemStore(), abci.Header{Height: 27}, nil, nil)
	ctx3 = context.WithValue(ctx3, ContextKeyCheckTx, true)

	//If we get to tx with incrementing sequence numbers we should be fine in the same block
	_, err = NonceTxMiddleware(state3, nonceTxBytes2,
		func(state3 diademchain.State, txBytes []byte, isCheckTx bool) (diademchain.TxHandlerResult, error) {
			return diademchain.TxHandlerResult{}, nil
		}, true,
	)
	require.Nil(t, err)
	NonceTxPostNonceMiddleware(state, nonceTxBytes, diademchain.TxHandlerResult{}, nil)

	//Try a deliverTx at same height it should be fine
	ctx3Dx := context.WithValue(context.Background(), ContextKeyOrigin, origin)
	state3Dx := diademchain.NewStoreState(ctx3Dx, store.NewMemStore(), abci.Header{Height: 27}, nil, nil)
	ctx3Dx = context.WithValue(ctx3Dx, ContextKeyCheckTx, true)

	_, err = NonceTxMiddleware(state3Dx, nonceTxBytes,
		func(state3 diademchain.State, txBytes []byte, isCheckTx bool) (diademchain.TxHandlerResult, error) {
			return diademchain.TxHandlerResult{}, nil
		}, false,
	)
	require.Nil(t, err)
	NonceTxPostNonceMiddleware(state, nonceTxBytes, diademchain.TxHandlerResult{}, nil)

	///--------------increase block height should kill cache
	//State is reset on every run
	ctx4 := context.WithValue(nil, ContextKeyOrigin, origin)
	state4 := diademchain.NewStoreState(ctx4, store.NewMemStore(), abci.Header{Height: 28}, nil, nil)

	//If we get to tx with incrementing sequence numbers we should be fine in the same block
	_, err = NonceTxMiddleware(state4, nonceTxBytes,
		func(state4 diademchain.State, txBytes []byte, isCheckTx bool) (diademchain.TxHandlerResult, error) {
			return diademchain.TxHandlerResult{}, nil
		}, true,
	)
	require.Nil(t, err)
	NonceTxPostNonceMiddleware(state, nonceTxBytes, diademchain.TxHandlerResult{}, nil)

}
