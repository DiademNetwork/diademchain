package diademchain

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"time"

	"github.com/diademnetwork/go-diadem/util"
	"github.com/diademnetwork/diademchain/eth/utils"
	"github.com/diademnetwork/diademchain/registry"

	"github.com/go-kit/kit/metrics"
	kitprometheus "github.com/go-kit/kit/metrics/prometheus"
	diadem "github.com/diademnetwork/go-diadem"
	"github.com/diademnetwork/go-diadem/plugin"
	"github.com/diademnetwork/go-diadem/types"
	"github.com/diademnetwork/diademchain/log"
	"github.com/diademnetwork/diademchain/store"
	stdprometheus "github.com/prometheus/client_golang/prometheus"
	abci "github.com/tendermint/tendermint/abci/types"
	"github.com/tendermint/tendermint/libs/common"

	dbm "github.com/tendermint/tendermint/libs/db"
)

type ReadOnlyState interface {
	store.KVReader
	Validators() []*diadem.Validator
	Block() types.BlockHeader
	// Release should free up any underlying system resources. Must be safe to invoke multiple times.
	Release()
	FeatureEnabled(string, bool) bool
}

type State interface {
	ReadOnlyState
	store.KVWriter
	Context() context.Context
	WithContext(ctx context.Context) State
	WithPrefix(prefix []byte) State
	SetFeature(string, bool)
}

type StoreState struct {
	ctx             context.Context
	store           store.KVStore
	block           types.BlockHeader
	validators      diadem.ValidatorSet
	getValidatorSet GetValidatorSet
}

var _ = State(&StoreState{})

func blockHeaderFromAbciHeader(header *abci.Header) types.BlockHeader {
	return types.BlockHeader{
		ChainID: header.ChainID,
		Height:  header.Height,
		Time:    header.Time.Unix(),
		NumTxs:  int32(header.NumTxs), //TODO this cast doesnt look right
		LastBlockID: types.BlockID{
			Hash: header.LastBlockId.Hash,
		},
		ValidatorsHash: header.ValidatorsHash,
		AppHash:        header.AppHash,
	}
}

func NewStoreState(
	ctx context.Context,
	store store.KVStore,
	block abci.Header,
	curBlockHash []byte,
	getValidatorSet GetValidatorSet,
) *StoreState {
	blockHeader := blockHeaderFromAbciHeader(&block)
	blockHeader.CurrentHash = curBlockHash
	return &StoreState{
		ctx:             ctx,
		store:           store,
		block:           blockHeader,
		validators:      diadem.NewValidatorSet(),
		getValidatorSet: getValidatorSet,
	}
}

func (c *StoreState) Range(prefix []byte) plugin.RangeData {
	return c.store.Range(prefix)
}

func (s *StoreState) Get(key []byte) []byte {
	return s.store.Get(key)
}

func (s *StoreState) Has(key []byte) bool {
	return s.store.Has(key)
}

func (s *StoreState) Validators() []*diadem.Validator {
	if (len(s.validators) == 0) && (s.getValidatorSet != nil) {
		validatorSet, err := s.getValidatorSet(s)
		if err != nil {
			panic(err)
		}
		// cache the validator set for the current state
		s.validators = validatorSet
	}
	return s.validators.Slice()
}

func (s *StoreState) Set(key, value []byte) {
	s.store.Set(key, value)
}

func (s *StoreState) Delete(key []byte) {
	s.store.Delete(key)
}

func (s *StoreState) Block() types.BlockHeader {
	return s.block
}

func (s *StoreState) Context() context.Context {
	return s.ctx
}

var (
	featurePrefix       = "feature"
	evmRootMapperPrefix = "evmdbmapper"
	evmRootKey          = "vmroot"
)

func featureKey(featureName string) []byte {
	return util.PrefixKey([]byte(featurePrefix), []byte(featureName))
}

// EvmDBMapperKey returns the root of Patricia tree at specific block height
func EvmDBMapperKey(blockHeight int64) []byte {
	b := make([]byte, 8)
	binary.LittleEndian.PutUint64(b, uint64(blockHeight))
	return util.PrefixKey([]byte(evmRootMapperPrefix), b)
}

func (s *StoreState) FeatureEnabled(name string, val bool) bool {

	data := s.store.Get(featureKey(name))
	if len(data) == 0 {
		return val
	}
	if bytes.Equal(data, []byte{1}) {
		return true
	}
	return false
}

func (s *StoreState) SetFeature(name string, val bool) {
	data := []byte{0}
	if val {
		data = []byte{1}
	}
	s.store.Set(featureKey(name), data)
}

func (s *StoreState) WithContext(ctx context.Context) State {
	return &StoreState{
		store:           s.store,
		block:           s.block,
		ctx:             ctx,
		validators:      s.validators,
		getValidatorSet: s.getValidatorSet,
	}
}

func (s *StoreState) WithPrefix(prefix []byte) State {
	return &StoreState{
		store:           store.PrefixKVStore(prefix, s.store),
		block:           s.block,
		ctx:             s.ctx,
		validators:      s.validators,
		getValidatorSet: s.getValidatorSet,
	}
}

func (s *StoreState) Release() {
	// noop
}

// StoreStateSnapshot is a read-only snapshot of the app state at particular point in time,
// it's unaffected by any changes to the app state. Multiple snapshots can exist at any one
// time, but each snapshot should only be accessed from one thread at a time. After a snapshot
// is no longer needed call Release() to free up underlying resources. Live snapshots may prevent
// the underlying DB from writing new data in the most space efficient manner, so aim to minimize
// their lifetime.
type StoreStateSnapshot struct {
	*StoreState
	storeSnapshot store.Snapshot
}

// TODO: Ideally StoreStateSnapshot should only implement ReadOnlyState interface, but that will
//       require updating a bunch of the existing State consumers to also handle ReadOnlyState.
var _ = State(&StoreStateSnapshot{})

// NewStoreStateSnapshot creates a new snapshot of the app state.
func NewStoreStateSnapshot(ctx context.Context, snap store.Snapshot, block abci.Header, curBlockHash []byte, getValidatorSet GetValidatorSet) *StoreStateSnapshot {
	return &StoreStateSnapshot{
		StoreState:    NewStoreState(ctx, &readOnlyKVStoreAdapter{snap}, block, curBlockHash, getValidatorSet),
		storeSnapshot: snap,
	}
}

// Release releases the underlying store snapshot, safe to call multiple times.
func (s *StoreStateSnapshot) Release() {
	if s.storeSnapshot != nil {
		s.storeSnapshot.Release()
		s.storeSnapshot = nil
	}
}

// For all the times you need a read-only store.KVStore but you only have a store.KVReader.
type readOnlyKVStoreAdapter struct {
	store.KVReader
}

func (s *readOnlyKVStoreAdapter) Set(key, value []byte) {
	panic("kvStoreSnapshotAdapter.Set not implemented")
}

func (s *readOnlyKVStoreAdapter) Delete(key []byte) {
	panic("kvStoreSnapshotAdapter.Delete not implemented")
}

type TxHandler interface {
	ProcessTx(state State, txBytes []byte, isCheckTx bool) (TxHandlerResult, error)
}

type TxHandlerFunc func(state State, txBytes []byte, isCheckTx bool) (TxHandlerResult, error)

type TxHandlerResult struct {
	Data             []byte
	ValidatorUpdates []abci.Validator
	Info             string
	// Tags to associate with the tx that produced this result. Tags can be used to filter txs
	// via the ABCI query interface (see https://godoc.org/github.com/tendermint/tendermint/libs/pubsub/query)
	Tags []common.KVPair
}

func (f TxHandlerFunc) ProcessTx(state State, txBytes []byte, isCheckTx bool) (TxHandlerResult, error) {
	return f(state, txBytes, isCheckTx)
}

type QueryHandler interface {
	Handle(state ReadOnlyState, path string, data []byte) ([]byte, error)
}

type OriginHandler interface {
	ValidateOrigin(input []byte, chainId string, currentBlockHeight int64) error
	Reset(currentBlockHeight int64)
}

type KarmaHandler interface {
	Upkeep() error
}

type ValidatorsManager interface {
	BeginBlock(abci.RequestBeginBlock, int64) error
	EndBlock(abci.RequestEndBlock) ([]abci.ValidatorUpdate, error)
}

type ChainConfigManager interface {
	EnableFeatures(blockHeight int64) error
}

type GetValidatorSet func(state State) (diadem.ValidatorSet, error)

type ValidatorsManagerFactoryFunc func(state State) (ValidatorsManager, error)

type ChainConfigManagerFactoryFunc func(state State) (ChainConfigManager, error)

type Application struct {
	lastBlockHeader abci.Header
	curBlockHeader  abci.Header
	curBlockHash    []byte
	Store           store.VersionedKVStore
	Init            func(State) error
	TxHandler
	QueryHandler
	EventHandler
	ReceiptHandlerProvider
	CreateValidatorManager   ValidatorsManagerFactoryFunc
	CreateChainConfigManager ChainConfigManagerFactoryFunc
	OriginHandler
	// Callback function used to construct a contract upkeep handler at the start of each block,
	// should return a nil handler when the contract upkeep feature is disabled.
	CreateContractUpkeepHandler func(state State) (KarmaHandler, error)
	GetValidatorSet             GetValidatorSet
	EventStore                  store.EventStore
	EvmDB                       dbm.DB
}

var _ abci.Application = &Application{}

//Metrics
var (
	deliverTxLatency    metrics.Histogram
	checkTxLatency      metrics.Histogram
	commitBlockLatency  metrics.Histogram
	requestCount        metrics.Counter
	committedBlockCount metrics.Counter
)

func init() {
	fieldKeys := []string{"method", "error"}
	requestCount = kitprometheus.NewCounterFrom(stdprometheus.CounterOpts{
		Namespace: "diademchain",
		Subsystem: "application",
		Name:      "request_count",
		Help:      "Number of requests received.",
	}, fieldKeys)
	deliverTxLatency = kitprometheus.NewSummaryFrom(stdprometheus.SummaryOpts{
		Namespace: "diademchain",
		Subsystem: "application",
		Name:      "delivertx_latency_microseconds",
		Help:      "Total duration of delivertx in microseconds.",
	}, fieldKeys)

	checkTxLatency = kitprometheus.NewSummaryFrom(stdprometheus.SummaryOpts{
		Namespace: "diademchain",
		Subsystem: "application",
		Name:      "checktx_latency_microseconds",
		Help:      "Total duration of checktx in microseconds.",
	}, fieldKeys)
	commitBlockLatency = kitprometheus.NewSummaryFrom(stdprometheus.SummaryOpts{
		Namespace: "diademchain",
		Subsystem: "application",
		Name:      "commit_block_latency_microseconds",
		Help:      "Total duration of commit block in microseconds.",
	}, fieldKeys)

	committedBlockCount = kitprometheus.NewCounterFrom(stdprometheus.CounterOpts{
		Namespace: "diademchain",
		Subsystem: "application",
		Name:      "block_count",
		Help:      "Number of committed blocks.",
	}, fieldKeys)
}

func (a *Application) Info(req abci.RequestInfo) abci.ResponseInfo {
	return abci.ResponseInfo{
		LastBlockAppHash: a.Store.Hash(),
		LastBlockHeight:  a.Store.Version(),
	}
}

func (a *Application) SetOption(req abci.RequestSetOption) abci.ResponseSetOption {
	return abci.ResponseSetOption{}
}

func (a *Application) InitChain(req abci.RequestInitChain) abci.ResponseInitChain {
	if a.height() != 1 {
		panic("state version is not 1")
	}

	state := NewStoreState(
		context.Background(),
		a.Store,
		abci.Header{},
		nil,
		a.GetValidatorSet,
	)

	if a.Init != nil {
		err := a.Init(state)
		if err != nil {
			panic(err)
		}
	}
	return abci.ResponseInitChain{}
}

func (a *Application) BeginBlock(req abci.RequestBeginBlock) abci.ResponseBeginBlock {
	block := req.Header
	if block.Height != a.height() {
		panic(fmt.Sprintf("app height %d doesn't match BeginBlock height %d", a.height(), block.Height))
	}

	a.curBlockHeader = block
	a.curBlockHash = req.Hash

	if a.CreateContractUpkeepHandler != nil {
		upkeepStoreTx := store.WrapAtomic(a.Store).BeginTx()
		upkeepState := NewStoreState(
			context.Background(),
			upkeepStoreTx,
			a.curBlockHeader,
			a.curBlockHash,
			a.GetValidatorSet,
		)
		contractUpkeepHandler, err := a.CreateContractUpkeepHandler(upkeepState)
		if err != nil {
			panic(err)
		}
		if contractUpkeepHandler != nil {
			if err := contractUpkeepHandler.Upkeep(); err != nil {
				panic(err)
			}
			upkeepStoreTx.Commit()
		}
	}

	a.OriginHandler.Reset(a.curBlockHeader.Height)

	storeTx := store.WrapAtomic(a.Store).BeginTx()
	state := NewStoreState(
		context.Background(),
		storeTx,
		a.curBlockHeader,
		nil,
		a.GetValidatorSet,
	)

	validatorManager, err := a.CreateValidatorManager(state)
	if err != registry.ErrNotFound {
		if err != nil {
			panic(err)
		}

		err = validatorManager.BeginBlock(req, a.height())
		if err != nil {
			panic(err)
		}
	}

	//Enable Features
	chainConfigManager, err := a.CreateChainConfigManager(state)
	if err != nil {
		panic(err)
	}
	if chainConfigManager != nil {
		if err := chainConfigManager.EnableFeatures(a.height()); err != nil {
			panic(err)
		}
	}

	storeTx.Commit()

	return abci.ResponseBeginBlock{}
}

func (a *Application) EndBlock(req abci.RequestEndBlock) abci.ResponseEndBlock {
	if req.Height != a.height() {
		panic(fmt.Sprintf("app height %d doesn't match EndBlock height %d", a.height(), req.Height))
	}

	storeTx := store.WrapAtomic(a.Store).BeginTx()
	state := NewStoreState(
		context.Background(),
		storeTx,
		a.curBlockHeader,
		nil,
		a.GetValidatorSet,
	)
	receiptHandler, err := a.ReceiptHandlerProvider.StoreAt(a.height(), state.FeatureEnabled(EvmTxReceiptsVersion2Feature, false))
	if err != nil {
		panic(err)
	}
	if err := receiptHandler.CommitBlock(state, a.height()); err != nil {
		storeTx.Rollback()
		// TODO: maybe panic instead?
		log.Error(fmt.Sprintf("aborted committing block receipts, %v", err.Error()))
	} else {
		storeTx.Commit()
	}

	storeTx = store.WrapAtomic(a.Store).BeginTx()
	state = NewStoreState(
		context.Background(),
		storeTx,
		a.curBlockHeader,
		nil,
		a.GetValidatorSet,
	)

	validatorManager, err := a.CreateValidatorManager(state)
	if err != registry.ErrNotFound {
		if err != nil {
			panic(err)
		}
		validators, err := validatorManager.EndBlock(req)
		if err != nil {
			panic(err)
		}

		storeTx.Commit()

		return abci.ResponseEndBlock{
			ValidatorUpdates: validators,
		}
	}
	return abci.ResponseEndBlock{
		ValidatorUpdates: []abci.ValidatorUpdate{},
	}
}

func (a *Application) CheckTx(txBytes []byte) abci.ResponseCheckTx {
	ok := abci.ResponseCheckTx{Code: abci.CodeTypeOK}

	var err error
	defer func(begin time.Time) {
		lvs := []string{"method", "CheckTx", "error", fmt.Sprint(err != nil)}
		checkTxLatency.With(lvs...).Observe(time.Since(begin).Seconds())
	}(time.Now())

	// If the chain is configured not to generate empty blocks then CheckTx may be called before
	// BeginBlock when the application restarts, which means that both curBlockHeader and
	// lastBlockHeader will be default initialized. Instead of invoking a contract method with
	// a vastly innacurate block header simply skip invoking the contract. This has the minor
	// disadvantage of letting an potentially invalid tx propagate to other nodes, but this should
	// only happen on node restarts, and only if the node doesn't receive any txs from it's peers
	// before a client sends it a tx.
	if a.curBlockHeader.Height == 0 {
		return ok
	}

	_, err = a.processTx(txBytes, true)
	if err != nil {
		log.Error(fmt.Sprintf("CheckTx: %s", err.Error()))
		return abci.ResponseCheckTx{Code: 1, Log: err.Error()}
	}

	return ok
}
func (a *Application) DeliverTx(txBytes []byte) abci.ResponseDeliverTx {
	var err error
	defer func(begin time.Time) {
		lvs := []string{"method", "DeliverTx", "error", fmt.Sprint(err != nil)}
		requestCount.With(lvs...).Add(1)
		deliverTxLatency.With(lvs...).Observe(time.Since(begin).Seconds())
	}(time.Now())

	r, err := a.processTx(txBytes, false)
	if err != nil {
		log.Error(fmt.Sprintf("DeliverTx: %s", err.Error()))
		return abci.ResponseDeliverTx{Code: 1, Log: err.Error()}
	}
	return abci.ResponseDeliverTx{Code: abci.CodeTypeOK, Data: r.Data, Tags: r.Tags, Info: r.Info}
}

func (a *Application) processTx(txBytes []byte, isCheckTx bool) (TxHandlerResult, error) {
	var err error
	//TODO we should be keeping this across multiple checktx, and only rolling back after they all complete
	// for now the nonce will have a special cache that it rolls back each block
	storeTx := store.WrapAtomic(a.Store).BeginTx()
	state := NewStoreState(
		context.Background(),
		storeTx,
		a.curBlockHeader,
		a.curBlockHash,
		a.GetValidatorSet,
	)

	if isCheckTx {
		err := a.OriginHandler.ValidateOrigin(txBytes, state.Block().ChainID, state.Block().Height)
		if err != nil {
			storeTx.Rollback()
			return TxHandlerResult{}, err
		}
	}

	receiptHandler, err := a.ReceiptHandlerProvider.StoreAt(a.height(), state.FeatureEnabled(EvmTxReceiptsVersion2Feature, false))
	if err != nil {
		panic(err)
	}
	r, err := a.TxHandler.ProcessTx(state, txBytes, isCheckTx)
	if err != nil {
		storeTx.Rollback()
		// TODO: save receipt & hash of failed EVM tx to node-local persistent cache (not app state)
		receiptHandler.DiscardCurrentReceipt()
		return r, err
	}

	if !isCheckTx {
		if r.Info == utils.CallEVM || r.Info == utils.DeployEvm {
			err := a.EventHandler.LegacyEthSubscriptionSet().EmitTxEvent(r.Data, r.Info)
			if err != nil {
				log.Error("Emit Tx Event error", "err", err)
			}
			reader, err := a.ReceiptHandlerProvider.ReaderAt(state.Block().Height, state.FeatureEnabled(EvmTxReceiptsVersion2Feature, false))
			if err != nil {
				log.Error("failed to load receipt", "height", state.Block().Height, "err", err)
			} else {
				if reader.GetCurrentReceipt() != nil {
					if err = a.EventHandler.EthSubscriptionSet().EmitTxEvent(reader.GetCurrentReceipt().TxHash); err != nil {
						log.Error("failed to load receipt", "err", err)
					}
				}
			}
			receiptHandler.CommitCurrentReceipt()
		}
		storeTx.Commit()
	}
	return r, nil
}

// Commit commits the current block
func (a *Application) Commit() abci.ResponseCommit {
	var err error
	defer func(begin time.Time) {
		lvs := []string{"method", "Commit", "error", fmt.Sprint(err != nil)}
		committedBlockCount.With(lvs...).Add(1)
		commitBlockLatency.With(lvs...).Observe(time.Since(begin).Seconds())
	}(time.Now())

	appHash, _, err := a.Store.SaveVersion()
	if err != nil {
		// Rollback evm.db by setting Patricia tree root to previous block
		previousBlockRoot := a.Store.Get(EvmDBMapperKey(a.curBlockHeader.Height - 1))
		a.Store.Set([]byte(evmRootKey), previousBlockRoot)
		panic(err)
	}

	height := a.curBlockHeader.GetHeight()
	go func(height int64, blockHeader abci.Header) {
		if err := a.EventHandler.EmitBlockTx(uint64(height), blockHeader.Time); err != nil {
			log.Error("Emit Block Event error", "err", err)
		}
		if err := a.EventHandler.LegacyEthSubscriptionSet().EmitBlockEvent(blockHeader); err != nil {
			log.Error("Emit Block Event error", "err", err)
		}
		if err := a.EventHandler.EthSubscriptionSet().EmitBlockEvent(blockHeader); err != nil {
			log.Error("Emit Block Event error", "err", err)
		}
	}(height, a.curBlockHeader)
	a.lastBlockHeader = a.curBlockHeader

	if err := a.Store.Prune(); err != nil {
		log.Error("failed to prune app.db", "err", err)
	}

	return abci.ResponseCommit{
		Data: appHash,
	}
}

func (a *Application) Query(req abci.RequestQuery) abci.ResponseQuery {
	if a.QueryHandler == nil {
		return abci.ResponseQuery{Code: 1, Log: "not implemented"}
	}

	result, err := a.QueryHandler.Handle(a.ReadOnlyState(), req.Path, req.Data)
	if err != nil {
		return abci.ResponseQuery{Code: 1, Log: err.Error()}
	}

	return abci.ResponseQuery{Code: abci.CodeTypeOK, Value: result}
}

func (a *Application) height() int64 {
	return a.Store.Version() + 1
}

func (a *Application) ReadOnlyState() State {
	// FIXME: Figure out a less ugly way to do this
	var readOnlyStore store.Snapshot
	// TODO: Caching store needs to be updated to handle real snapshots from MultiReaderIAVLStore
	if cachingStore, ok := (a.Store.(*store.CachingStore)); ok {
		readOnlyStore = store.NewReadOnlyCachingStore(cachingStore)
	} else {
		readOnlyStore = a.Store.GetSnapshot()
	}

	// TODO: the store snapshot should be created atomically, otherwise the block header might
	//       not match the state... need to figure out why this hasn't spectacularly failed already
	return NewStoreStateSnapshot(
		nil,
		readOnlyStore,
		a.lastBlockHeader,
		nil, // TODO: last block hash!
		a.GetValidatorSet,
	)
}
