package auth

import (
	"context"
	"errors"
	"fmt"

	"github.com/go-kit/kit/metrics"
	kitprometheus "github.com/go-kit/kit/metrics/prometheus"
	"github.com/gogo/protobuf/proto"
	stdprometheus "github.com/prometheus/client_golang/prometheus"
	"golang.org/x/crypto/ed25519"

	diadem "github.com/diademnetwork/go-diadem"
	"github.com/diademnetwork/go-diadem/util"
	"github.com/diademnetwork/diademchain"
)

var (
	nonceErrorCount metrics.Counter
)

func init() {
	nonceErrorCount = kitprometheus.NewCounterFrom(stdprometheus.CounterOpts{
		Namespace: "diademchain",
		Subsystem: "middleware",
		Name:      "nonce_error",
		Help:      "Number of invalid nonces.",
	}, []string{})
}

type contextKey string

func (c contextKey) String() string {
	return "auth " + string(c)
}

var (
	ContextKeyOrigin  = contextKey("origin")
	ContextKeyCheckTx = contextKey("CheckTx")
)

func Origin(ctx context.Context) diadem.Address {
	return ctx.Value(ContextKeyOrigin).(diadem.Address)
}

var SignatureTxMiddleware = diademchain.TxMiddlewareFunc(func(
	state diademchain.State,
	txBytes []byte,
	next diademchain.TxHandlerFunc,
	isCheckTx bool,
) (diademchain.TxHandlerResult, error) {
	var r diademchain.TxHandlerResult

	var tx SignedTx
	err := proto.Unmarshal(txBytes, &tx)
	if err != nil {
		return r, err
	}

	origin, err := GetOrigin(tx, state.Block().ChainID)
	if err != nil {
		return r, err
	}

	ctx := context.WithValue(state.Context(), ContextKeyOrigin, origin)
	return next(state.WithContext(ctx), tx.Inner, isCheckTx)
})

func GetOrigin(tx SignedTx, chainId string) (diadem.Address, error) {
	if len(tx.PublicKey) != ed25519.PublicKeySize {
		return diadem.Address{}, errors.New("invalid public key length")
	}

	if len(tx.Signature) != ed25519.SignatureSize {
		return diadem.Address{}, errors.New("invalid signature ed25519 signature size length")
	}

	if !ed25519.Verify(tx.PublicKey, tx.Inner, tx.Signature) {
		return diadem.Address{}, errors.New("invalid signature ed25519 verify")
	}

	return diadem.Address{
		ChainID: chainId,
		Local:   diadem.LocalAddressFromPublicKey(tx.PublicKey),
	}, nil
}

func nonceKey(addr diadem.Address) []byte {
	return util.PrefixKey([]byte("nonce"), addr.Bytes())
}

func Nonce(state diademchain.ReadOnlyState, addr diadem.Address) uint64 {
	return diademchain.NewSequence(nonceKey(addr)).Value(state)
}

type NonceHandler struct {
	nonceCache map[string]uint64
	lastHeight int64
}

func (n *NonceHandler) Nonce(
	state diademchain.State,
	txBytes []byte,
	next diademchain.TxHandlerFunc,
	isCheckTx bool,
) (diademchain.TxHandlerResult, error) {
	var r diademchain.TxHandlerResult
	origin := Origin(state.Context())
	if origin.IsEmpty() {
		return r, errors.New("transaction has no origin [nonce]")
	}
	if n.lastHeight != state.Block().Height {
		n.lastHeight = state.Block().Height
		n.nonceCache = make(map[string]uint64)
		//clear the cache for each block
	}
	seq := diademchain.NewSequence(nonceKey(origin)).Next(state)

	var tx NonceTx
	err := proto.Unmarshal(txBytes, &tx)
	if err != nil {
		return r, err
	}

	//TODO nonce cache is temporary until we have a separate atomic state for the entire checktx flow
	cacheSeq := n.nonceCache[origin.String()]
	//If we have a client send multiple transactions in a single block we can run into this problem
	if cacheSeq != 0 && isCheckTx { //only run this code during checktx
		seq = cacheSeq
	} else {
		n.nonceCache[origin.String()] = seq
	}

	if tx.Sequence != seq {
		nonceErrorCount.Add(1)
		return r, fmt.Errorf("sequence number does not match expected %d got %d", seq, tx.Sequence)
	}

	return next(state, tx.Inner, isCheckTx)
}

func (n *NonceHandler) IncNonce(state diademchain.State,
	txBytes []byte,
	result diademchain.TxHandlerResult,
	postcommit diademchain.PostCommitHandler,
) error {
	origin := Origin(state.Context())
	if origin.IsEmpty() {
		return errors.New("transaction has no origin [IncNonce]")
	}

	//We only increment the nonce if the transaction is successful
	//There are situations in checktx where we may not have committed the transaction to the statestore yet
	n.nonceCache[origin.String()] = n.nonceCache[origin.String()] + 1
	return nil
}

var NonceTxHandler = NonceHandler{nonceCache: make(map[string]uint64), lastHeight: 0}

var NonceTxPostNonceMiddleware = diademchain.PostCommitMiddlewareFunc(NonceTxHandler.IncNonce)
var NonceTxMiddleware = diademchain.TxMiddlewareFunc(NonceTxHandler.Nonce)
