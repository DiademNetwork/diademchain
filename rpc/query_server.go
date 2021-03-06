package rpc

import (
	"encoding/hex"
	"fmt"
	"math/big"
	"strconv"
	"strings"

	"github.com/gorilla/websocket"

	"github.com/gogo/protobuf/proto"
	diadem "github.com/diademnetwork/go-diadem"
	"github.com/diademnetwork/go-diadem/plugin"
	"github.com/diademnetwork/go-diadem/plugin/contractpb"
	"github.com/diademnetwork/go-diadem/plugin/types"
	"github.com/diademnetwork/go-diadem/vm"
	"github.com/diademnetwork/diademchain"
	"github.com/diademnetwork/diademchain/auth"
	"github.com/diademnetwork/diademchain/config"
	"github.com/diademnetwork/diademchain/eth/polls"
	"github.com/diademnetwork/diademchain/eth/query"
	"github.com/diademnetwork/diademchain/eth/subs"
	levm "github.com/diademnetwork/diademchain/evm"
	"github.com/diademnetwork/diademchain/log"
	lcp "github.com/diademnetwork/diademchain/plugin"
	hsmpv "github.com/diademnetwork/diademchain/privval/hsm"
	registry "github.com/diademnetwork/diademchain/registry/factory"
	"github.com/diademnetwork/diademchain/rpc/eth"
	"github.com/diademnetwork/diademchain/store"
	lvm "github.com/diademnetwork/diademchain/vm"
	sha3 "github.com/miguelmota/go-solidity-sha3"
	pubsub "github.com/phonkee/go-pubsub"
	"github.com/pkg/errors"
	dbm "github.com/tendermint/tendermint/libs/db"
	ctypes "github.com/tendermint/tendermint/rpc/core/types"
	rpctypes "github.com/tendermint/tendermint/rpc/lib/types"
)

// StateProvider interface is used by QueryServer to access the read-only application state
type StateProvider interface {
	ReadOnlyState() diademchain.State
}

// QueryServer provides the ability to query the current state of the DAppChain via RPC.
//
// Contract state can be queried via:
// - POST request of a JSON-RPC 2.0 object to "/" endpoint:
//   {
//     "jsonrpc": "2.0",
//     "method": "query",
//     "params": {
//       "contract": "0x000000000000000000",
//       "query": { /* query params */ }
//     },
//     "id": "123456789"
//   }
// - POST request to "/query" endpoint with form-encoded contract & query params.
//
// Contract query requests must contain two parameters:
// - contract: the address of the contract to be queried (hex encoded string), and
// - query: a JSON object containing the query parameters, the Diadem SDK makes no assumptions about
//          the structure of the object, it is entirely up to the contract author to define the
//          query interface.
//
// The JSON-RPC 2.0 response object will contain the query result as a JSON object:
// {
//   "jsonrpc": "2.0",
//   "result": { /* query result */ },
//   "id": "123456789"
// }
//
// On error the JSON-RPC 2.0 response object will look similar to this:
// {
//   "jsonrpc": "2.0",
//   "error": {
//	   "code": -32603,
//	   "message": "Internal error",
//	   "data": "invalid query"
//   },
//   "id": "123456789"
// }
//
// The nonce associated with a particular signer can be obtained via:
// - GET request to /nonce?key="<hex-encoded-public-key-of-signer>"
// - POST request of a JSON-RPC 2.0 object to "/" endpoint:
//   {
//     "jsonrpc": "2.0",
//     "method": "nonce",
//     "params": {
//       "key": "hex-encoded-public-key-of-signer",
//     },
//     "id": "123456789"
//   }
// - POST request to "/nonce" endpoint with form-encoded key param.
type QueryServer struct {
	StateProvider
	ChainID                string
	Loader                 lcp.Loader
	Subscriptions          *diademchain.SubscriptionSet
	EthSubscriptions       *subs.EthSubscriptionSet
	EthLegacySubscriptions *subs.LegacyEthSubscriptionSet
	EthPolls               polls.EthSubscriptions
	CreateRegistry         registry.RegistryFactoryFunc
	// If this is nil the EVM won't have access to any account balances.
	NewABMFactory lcp.NewAccountBalanceManagerFactoryFunc
	diademchain.ReceiptHandlerProvider
	RPCListenAddress string
	store.BlockStore
	EventStore store.EventStore
	EvmDB      dbm.DB
	AuthCfg    *auth.Config
}

var _ QueryService = &QueryServer{}

// Query returns data of given contract from the application states
// The contract parameter should be a hex-encoded local address prefixed by 0x
func (s *QueryServer) Query(caller, contract string, query []byte, vmType vm.VMType) ([]byte, error) {
	var callerAddr diadem.Address
	var err error
	if len(caller) == 0 {
		callerAddr = diadem.RootAddress(s.ChainID)
	} else {
		callerAddr, err = diadem.ParseAddress(caller)
		if err != nil {
			return nil, err
		}
	}

	localContractAddr, err := decodeHexAddress(contract)
	if err != nil {
		return nil, err
	}
	contractAddr := diadem.Address{
		ChainID: s.ChainID,
		Local:   localContractAddr,
	}

	if vmType == lvm.VMType_PLUGIN {
		return s.queryPlugin(callerAddr, contractAddr, query)
	} else {
		return s.queryEvm(callerAddr, contractAddr, query)
	}
}

func (s *QueryServer) QueryEnv() (*config.EnvInfo, error) {
	cfg, err := config.ParseConfig()
	if err != nil {
		return nil, err
	}

	gen, err := config.ReadGenesis(cfg.GenesisPath())
	if err != nil {
		return nil, err
	}

	envir := config.Env{
		Version:      diademchain.FullVersion(),
		Build:        diademchain.Build,
		BuildVariant: diademchain.BuildVariant,
		GitSha:       diademchain.GitSHA,
		GoDiadem:       diademchain.GoDiademGitSHA,
		GoEthereum:   diademchain.EthGitSHA,
		GoPlugin:     diademchain.HashicorpGitSHA,
		PluginPath:   cfg.PluginsPath(),
		Peers:        cfg.Peers,
	}

	// scrub the HSM config just in case
	cfg.HsmConfig = &hsmpv.HsmConfig{
		HsmEnabled: cfg.HsmConfig.HsmEnabled,
		HsmDevType: cfg.HsmConfig.HsmDevType,
	}

	envInfo := config.EnvInfo{
		Env:         envir,
		DiademGenesis: *gen,
		DiademConfig:  *cfg,
	}

	return &envInfo, err
}

func (s *QueryServer) queryPlugin(caller, contract diadem.Address, query []byte) ([]byte, error) {
	snapshot := s.StateProvider.ReadOnlyState()
	defer snapshot.Release()

	callerAddr, err := auth.ResolveAccountAddress(caller, snapshot, s.AuthCfg, s.createAddressMapperCtx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to resolve account address")
	}

	vm := lcp.NewPluginVM(
		s.Loader,
		snapshot,
		s.EvmDB,
		s.CreateRegistry(snapshot),
		nil,
		log.Default,
		s.NewABMFactory,
		nil,
		nil,
	)
	req := &plugin.Request{
		ContentType: plugin.EncodingType_PROTOBUF3,
		Accept:      plugin.EncodingType_PROTOBUF3,
		Body:        query,
	}
	reqBytes, err := proto.Marshal(req)
	if err != nil {
		return nil, err
	}

	respBytes, err := vm.StaticCall(callerAddr, contract, reqBytes)
	if err != nil {
		return nil, err
	}
	resp := &plugin.Response{}
	err = proto.Unmarshal(respBytes, resp)
	if err != nil {
		return nil, err
	}
	return resp.Body, nil
}

func (s *QueryServer) queryEvm(caller, contract diadem.Address, query []byte) ([]byte, error) {
	snapshot := s.StateProvider.ReadOnlyState()
	defer snapshot.Release()

	callerAddr, err := auth.ResolveAccountAddress(caller, snapshot, s.AuthCfg, s.createAddressMapperCtx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to resolve account address")
	}

	var createABM levm.AccountBalanceManagerFactoryFunc
	if s.NewABMFactory != nil {
		pvm := lcp.NewPluginVM(
			s.Loader,
			snapshot,
			s.EvmDB,
			s.CreateRegistry(snapshot),
			nil,
			log.Default,
			s.NewABMFactory,
			nil,
			nil,
		)
		createABM, err = s.NewABMFactory(pvm)
		if err != nil {
			return nil, err
		}
	}
	vm := levm.NewDiademVm(snapshot, s.EvmDB, nil, nil, createABM, false)
	return vm.StaticCall(callerAddr, contract, query)
}

// https://github.com/ethereum/wiki/wiki/JSON-RPC#eth_call
func (s QueryServer) EthCall(query eth.JsonTxCallObject, block eth.BlockHeight) (resp eth.Data, err error) {
	var caller diadem.Address
	if len(query.From) > 0 {
		caller, err = eth.DecDataToAddress(s.ChainID, query.From)
		if err != nil {
			return resp, err
		}
	}
	contract, err := eth.DecDataToAddress(s.ChainID, query.To)
	if err != nil {
		return resp, err
	}
	data, err := eth.DecDataToBytes(query.Data)
	if err != nil {
		return resp, err
	}
	bytes, err := s.queryEvm(caller, contract, data)
	return eth.EncBytes(bytes), err
}

// GetCode returns the runtime byte-code of a contract running on a DAppChain's EVM.
// Gives an error for non-EVM contracts.
// contract - address of the contract in the form of a string. (Use diadem.Address.String() to convert)
// return []byte - runtime bytecode of the contract.
func (s *QueryServer) GetEvmCode(contract string) ([]byte, error) {
	contractAddr, err := diadem.ParseAddress(contract)
	if err != nil {
		return nil, err
	}

	snapshot := s.StateProvider.ReadOnlyState()
	defer snapshot.Release()

	vm := levm.NewDiademVm(snapshot, s.EvmDB, nil, nil, nil, false)
	return vm.GetCode(contractAddr)
}

func (s *QueryServer) EthGetCode(address eth.Data, block eth.BlockHeight) (eth.Data, error) {
	addr, err := eth.DecDataToAddress(s.ChainID, address)
	if err != nil {
		return "", errors.Wrapf(err, "decoding input address parameter %v", address)
	}

	snapshot := s.StateProvider.ReadOnlyState()
	defer snapshot.Release()

	evm := levm.NewDiademVm(snapshot, s.EvmDB, nil, nil, nil, false)
	code, err := evm.GetCode(addr)
	if err != nil {
		return "", err
	}
	return eth.EncBytes(code), nil
}

// Attempts to construct the context of the Address Mapper contract.
func (s *QueryServer) createAddressMapperCtx(state diademchain.State) (contractpb.Context, error) {
	vm := lcp.NewPluginVM(
		s.Loader,
		state,
		s.EvmDB,
		s.CreateRegistry(state),
		nil, // event handler
		log.Default,
		s.NewABMFactory,
		nil, // receipt writer
		nil, // receipt reader
	)

	ctx, err := lcp.NewInternalContractContext("addressmapper", vm)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create Address Mapper context")
	}
	return ctx, nil
}

// Nonce returns the nonce of the last committed tx sent by the given account.
// NOTE: Either the key or the account must be provided. The account (if not empty) is used in
//       preference to the key.
func (s *QueryServer) Nonce(key, account string) (uint64, error) {
	var addr diadem.Address

	if key != "" && account == "" {
		k, err := hex.DecodeString(key)
		if err != nil {
			return 0, err
		}
		addr = diadem.Address{
			ChainID: s.ChainID,
			Local:   diadem.LocalAddressFromPublicKey(k),
		}
	} else if account != "" {
		var err error
		addr, err = diadem.ParseAddress(account)
		if err != nil {
			return 0, err
		}
	} else {
		return 0, errors.New("no key or account specified")
	}

	snapshot := s.StateProvider.ReadOnlyState()
	defer snapshot.Release()

	resolvedAddr, err := auth.ResolveAccountAddress(addr, snapshot, s.AuthCfg, s.createAddressMapperCtx)
	if err != nil {
		return 0, errors.Wrap(err, "failed to resolve account address")
	}

	return auth.Nonce(snapshot, resolvedAddr), nil
}

func (s *QueryServer) Resolve(name string) (string, error) {
	snapshot := s.StateProvider.ReadOnlyState()
	defer snapshot.Release()

	reg := s.CreateRegistry(snapshot)
	addr, err := reg.Resolve(name)
	if err != nil {
		return "", err
	}
	return addr.String(), nil
}

func decodeHexAddress(s string) ([]byte, error) {
	if !strings.HasPrefix(s, "0x") {
		return nil, errors.New("string has no hex prefix")
	}

	return hex.DecodeString(s[2:])
}

type WSEmptyResult struct{}

func writer(ctx rpctypes.WSRPCContext, subs *diademchain.SubscriptionSet) pubsub.SubscriberFunc {
	clientCtx := ctx
	log.Debug("Adding handler", "remote", clientCtx.GetRemoteAddr())
	return func(msg pubsub.Message) {
		log.Debug("Received published message", "msg", msg.Body(), "remote", clientCtx.GetRemoteAddr())
		defer func() {
			if r := recover(); r != nil {
				log.Error("Caught: WSEvent handler routine panic", "error", r)
				err := fmt.Errorf("Caught: WSEvent handler routine panic")
				clientCtx.WriteRPCResponse(rpctypes.RPCInternalError(rpctypes.JSONRPCStringID(""), err))
				go subs.Purge(clientCtx.GetRemoteAddr())
			}
		}()
		resp := rpctypes.RPCResponse{
			JSONRPC: "2.0",
			ID:      rpctypes.JSONRPCStringID("0"),
		}
		resp.Result = msg.Body()
		clientCtx.TryWriteRPCResponse(resp)
	}
}

func (s *QueryServer) Subscribe(wsCtx rpctypes.WSRPCContext, topics []string) (*WSEmptyResult, error) {
	if len(topics) == 0 {
		topics = append(topics, "contract")
	}
	caller := wsCtx.GetRemoteAddr()
	sub, existed := s.Subscriptions.For(caller)

	if !existed {
		sub.Do(writer(wsCtx, s.Subscriptions))
	}
	s.Subscriptions.AddSubscription(caller, topics)
	return &WSEmptyResult{}, nil
}

func (s *QueryServer) UnSubscribe(wsCtx rpctypes.WSRPCContext, topic string) (*WSEmptyResult, error) {
	s.Subscriptions.Remove(wsCtx.GetRemoteAddr(), topic)
	return &WSEmptyResult{}, nil
}

func ethWriter(ctx rpctypes.WSRPCContext, subs *subs.LegacyEthSubscriptionSet) pubsub.SubscriberFunc {
	clientCtx := ctx
	log.Debug("Adding handler", "remote", clientCtx.GetRemoteAddr())
	return func(msg pubsub.Message) {
		log.Debug("Received published message", "msg", msg.Body(), "remote", clientCtx.GetRemoteAddr())
		defer func() {
			if r := recover(); r != nil {
				log.Error("Caught: WSEvent handler routine panic", "error", r)
				err := fmt.Errorf("Caught: WSEvent handler routine panic")
				clientCtx.WriteRPCResponse(rpctypes.RPCInternalError(rpctypes.JSONRPCStringID(""), err))
				go subs.Purge(clientCtx.GetRemoteAddr())
			}
		}()
		ethMsg := types.EthMessage{}
		if err := proto.Unmarshal(msg.Body(), &ethMsg); err != nil {
			return
		}
		resp := rpctypes.RPCResponse{
			JSONRPC: "2.0",
			ID:      rpctypes.JSONRPCStringID(ethMsg.Id),
		}
		resp.Result = ethMsg.Body
		clientCtx.TryWriteRPCResponse(resp)
	}
}

func (s *QueryServer) EvmSubscribe(wsCtx rpctypes.WSRPCContext, method, filter string) (string, error) {
	caller := wsCtx.GetRemoteAddr()
	sub, id := s.EthLegacySubscriptions.For(caller)
	sub.Do(ethWriter(wsCtx, s.EthLegacySubscriptions))
	err := s.EthLegacySubscriptions.AddSubscription(id, method, filter)
	if err != nil {
		return "", err
	}
	return id, nil
}

func (s *QueryServer) EvmUnSubscribe(id string) (bool, error) {
	s.EthLegacySubscriptions.Remove(id)
	return true, nil
}

func (s *QueryServer) EvmTxReceipt(txHash []byte) ([]byte, error) {
	snapshot := s.StateProvider.ReadOnlyState()
	defer snapshot.Release()

	r, err := s.ReceiptHandlerProvider.ReaderAt(snapshot.Block().Height, snapshot.FeatureEnabled(diademchain.EvmTxReceiptsVersion2Feature, false))
	if err != nil {
		return nil, err
	}
	txReceipt, err := r.GetReceipt(snapshot, txHash)
	if err != nil {
		return nil, errors.Wrap(err, "get receipt")
	}

	if len(txReceipt.Logs) > 0 {
		height := int64(txReceipt.BlockNumber)
		var blockResult *ctypes.ResultBlock
		blockResult, err := s.BlockStore.GetBlockByHeight(&height)
		if err != nil {
			return nil, errors.Wrapf(err, "get block %d", height)
		}
		timestamp := blockResult.Block.Header.Time.Unix()

		for i := 0; i < len(txReceipt.Logs); i++ {
			txReceipt.Logs[i].BlockTime = timestamp
		}
	}
	return proto.Marshal(&txReceipt)
}

func (s *QueryServer) ContractEvents(fromBlock uint64, toBlock uint64, contractName string) (*types.ContractEventsResult, error) {
	if s.EventStore == nil {
		return nil, errors.New("event store is not available")
	}

	if fromBlock == 0 {
		return nil, fmt.Errorf("fromBlock not specified")
	}

	if toBlock == 0 {
		toBlock = fromBlock
	}

	if toBlock < fromBlock {
		return nil, fmt.Errorf("toBlock must be equal or greater than")
	}

	// default to max 20 blocks for now.
	maxRange := uint64(20)

	if toBlock-fromBlock > maxRange {
		return nil, fmt.Errorf("range exceeded, maximum range: %v", maxRange)
	}

	filter := store.EventFilter{
		FromBlock: fromBlock,
		ToBlock:   toBlock,
		Contract:  contractName,
	}
	events, err := s.EventStore.FilterEvents(filter)
	if err != nil {
		return nil, err
	}

	return &types.ContractEventsResult{
		Events:    events,
		FromBlock: fromBlock,
		ToBlock:   toBlock,
	}, nil
}

// Takes a filter and returns a list of data relative to transactions that satisfies the filter
// Used to support eth_getLogs
// https://github.com/ethereum/wiki/wiki/JSON-RPC#eth_getlogs
func (s *QueryServer) GetEvmLogs(filter string) ([]byte, error) {
	snapshot := s.StateProvider.ReadOnlyState()
	defer snapshot.Release()

	r, err := s.ReceiptHandlerProvider.ReaderAt(snapshot.Block().Height, snapshot.FeatureEnabled(diademchain.EvmTxReceiptsVersion2Feature, false))
	if err != nil {
		return nil, err
	}
	return query.DeprecatedQueryChain(filter, s.BlockStore, snapshot, r)
}

// Sets up new filter for polling
// https://github.com/ethereum/wiki/wiki/JSON-RPC#eth_newfilter
func (s *QueryServer) NewEvmFilter(filter string) (string, error) {
	snapshot := s.StateProvider.ReadOnlyState()
	defer snapshot.Release()

	return s.EthPolls.LegacyAddLogPoll(filter, uint64(snapshot.Block().Height))
}

// https://github.com/ethereum/wiki/wiki/JSON-RPC#eth_newblockfilter
func (s *QueryServer) NewBlockEvmFilter() (string, error) {
	snapshot := s.StateProvider.ReadOnlyState()
	defer snapshot.Release()

	return s.EthPolls.AddBlockPoll(uint64(snapshot.Block().Height)), nil
}

// https://github.com/ethereum/wiki/wiki/JSON-RPC#eth_newpendingtransactionfilter
func (s *QueryServer) NewPendingTransactionEvmFilter() (string, error) {
	snapshot := s.StateProvider.ReadOnlyState()
	defer snapshot.Release()

	return s.EthPolls.AddTxPoll(uint64(snapshot.Block().Height)), nil
}

// Get the logs since last poll
// https://github.com/ethereum/wiki/wiki/JSON-RPC#eth_getfilterchanges
func (s *QueryServer) GetEvmFilterChanges(id string) ([]byte, error) {
	snapshot := s.StateProvider.ReadOnlyState()
	defer snapshot.Release()

	r, err := s.ReceiptHandlerProvider.ReaderAt(snapshot.Block().Height, snapshot.FeatureEnabled(diademchain.EvmTxReceiptsVersion2Feature, false))
	if err != nil {
		return nil, err
	}
	// TODO: Reading from the TM block store could take a while, might be more efficient to release
	//       the current snapshot and get a new one after pulling out whatever we need from the TM
	//       block store.
	return s.EthPolls.LegacyPoll(s.BlockStore, snapshot, id, r)
}

// Forget the filter.
// https://github.com/ethereum/wiki/wiki/JSON-RPC#eth_uninstallfilter
func (s *QueryServer) UninstallEvmFilter(id string) (bool, error) {
	s.EthPolls.Remove(id)
	return true, nil
}

// https://github.com/ethereum/wiki/wiki/JSON-RPC#eth_blocknumber
func (s *QueryServer) EthBlockNumber() (eth.Quantity, error) {
	snapshot := s.StateProvider.ReadOnlyState()
	defer snapshot.Release()

	return eth.EncInt(snapshot.Block().Height), nil
}

// https://github.com/ethereum/wiki/wiki/JSON-RPC#eth_blocknumber
func (s *QueryServer) GetBlockHeight() (int64, error) {
	snapshot := s.StateProvider.ReadOnlyState()
	defer snapshot.Release()

	return snapshot.Block().Height - 1, nil
}

// https://github.com/ethereum/wiki/wiki/JSON-RPC#eth_getblockbynumber
func (s *QueryServer) GetEvmBlockByNumber(number string, full bool) ([]byte, error) {
	snapshot := s.StateProvider.ReadOnlyState()
	defer snapshot.Release()

	r, err := s.ReceiptHandlerProvider.ReaderAt(snapshot.Block().Height, snapshot.FeatureEnabled(diademchain.EvmTxReceiptsVersion2Feature, false))
	if err != nil {
		return nil, err
	}
	switch number {
	case "latest":
		return query.DeprecatedGetBlockByNumber(s.BlockStore, snapshot, snapshot.Block().Height-1, full, r)
	case "pending":
		return query.DeprecatedGetBlockByNumber(s.BlockStore, snapshot, snapshot.Block().Height, full, r)
	default:
		height, err := strconv.ParseInt(number, 10, 64)
		if err != nil {
			return nil, err
		}
		return query.DeprecatedGetBlockByNumber(s.BlockStore, snapshot, int64(height), full, r)
	}
}

// https://github.com/ethereum/wiki/wiki/JSON-RPC#eth_getblockbyhash
func (s *QueryServer) GetEvmBlockByHash(hash []byte, full bool) ([]byte, error) {
	snapshot := s.StateProvider.ReadOnlyState()
	defer snapshot.Release()

	r, err := s.ReceiptHandlerProvider.ReaderAt(snapshot.Block().Height, snapshot.FeatureEnabled(diademchain.EvmTxReceiptsVersion2Feature, false))
	if err != nil {
		return nil, err
	}
	return query.DeprecatedGetBlockByHash(s.BlockStore, snapshot, hash, full, r)
}

// https://github.com/ethereum/wiki/wiki/JSON-RPC#eth_gettransactionbyhash
func (s QueryServer) GetEvmTransactionByHash(txHash []byte) (resp []byte, err error) {
	snapshot := s.StateProvider.ReadOnlyState()
	defer snapshot.Release()

	r, err := s.ReceiptHandlerProvider.ReaderAt(snapshot.Block().Height, snapshot.FeatureEnabled(diademchain.EvmTxReceiptsVersion2Feature, false))
	if err != nil {
		return resp, err
	}
	return query.DeprecatedGetTxByHash(snapshot, txHash, r)
}

func (s *QueryServer) EthGetBlockByNumber(block eth.BlockHeight, full bool) (resp eth.JsonBlockObject, err error) {
	if block == "0x0" {
		return eth.GetBlockZero(), nil
	}

	snapshot := s.StateProvider.ReadOnlyState()
	defer snapshot.Release()

	height, err := eth.DecBlockHeight(snapshot.Block().Height, block)
	if err != nil {
		return resp, err
	}
	r, err := s.ReceiptHandlerProvider.ReaderAt(snapshot.Block().Height, snapshot.FeatureEnabled(diademchain.EvmTxReceiptsVersion2Feature, false))
	if err != nil {
		return resp, err
	}
	// TODO: Reading from the TM block store could take a while, might be more efficient to release
	//       the current snapshot and get a new one after pulling out whatever we need from the TM
	//       block store.
	blockResult, err := query.GetBlockByNumber(s.BlockStore, snapshot, int64(height), full, r)
	if err != nil {
		return resp, err
	}

	if block == "0x1" && blockResult.ParentHash == "0x0" {
		blockResult.ParentHash = "0x0000000000000000000000000000000000000000000000000000000000000001"
	}

	return blockResult, err
}

// https://github.com/ethereum/wiki/wiki/JSON-RPC#eth_gettransactionreceipt
func (s *QueryServer) EthGetTransactionReceipt(hash eth.Data) (resp eth.JsonTxReceipt, err error) {
	txHash, err := eth.DecDataToBytes(hash)
	if err != nil {
		return resp, err
	}

	snapshot := s.StateProvider.ReadOnlyState()
	defer snapshot.Release()

	r, err := s.ReceiptHandlerProvider.ReaderAt(snapshot.Block().Height, snapshot.FeatureEnabled(diademchain.EvmTxReceiptsVersion2Feature, false))
	if err != nil {
		return resp, err
	}
	txReceipt, err := r.GetReceipt(snapshot, txHash)
	if err != nil {
		return resp, err
	}

	// accessing the TM block store might take a while and we don't need the snapshot anymore
	snapshot.Release()

	if len(txReceipt.Logs) > 0 {
		height := int64(txReceipt.BlockNumber)
		var blockResult *ctypes.ResultBlock
		blockResult, err := s.BlockStore.GetBlockByHeight(&height)
		if err != nil {
			return resp, err
		}
		timestamp := blockResult.Block.Header.Time.Unix()

		for i := 0; i < len(txReceipt.Logs); i++ {
			txReceipt.Logs[i].BlockTime = timestamp
		}
	}

	return eth.EncTxReceipt(txReceipt), nil
}

// https://github.com/ethereum/wiki/wiki/JSON-RPC#eth_getblocktransactioncountbyhash
func (s *QueryServer) EthGetBlockTransactionCountByHash(hash eth.Data) (txCount eth.Quantity, err error) {
	blockHash, err := eth.DecDataToBytes(hash)
	if err != nil {
		return txCount, err
	}

	snapshot := s.StateProvider.ReadOnlyState()
	defer snapshot.Release()

	height, err := query.GetBlockHeightFromHash(s.BlockStore, snapshot, blockHash)
	if err != nil {
		return txCount, err
	}
	count, err := query.GetNumEvmTxBlock(s.BlockStore, snapshot, height)
	if err != nil {
		return txCount, err
	}
	return eth.EncUint(count), nil
}

// https://github.com/ethereum/wiki/wiki/JSON-RPC#eth_getblocktransactioncountbynumber
func (s *QueryServer) EthGetBlockTransactionCountByNumber(block eth.BlockHeight) (txCount eth.Quantity, err error) {
	snapshot := s.StateProvider.ReadOnlyState()
	defer snapshot.Release()

	height, err := eth.DecBlockHeight(snapshot.Block().Height, block)
	if err != nil {
		return txCount, err
	}
	count, err := query.GetNumEvmTxBlock(s.BlockStore, snapshot, int64(height))
	if err != nil {
		return txCount, err
	}
	return eth.EncUint(count), nil
}

// https://github.com/ethereum/wiki/wiki/JSON-RPC#eth_getblockbyhash
func (s *QueryServer) EthGetBlockByHash(hash eth.Data, full bool) (resp eth.JsonBlockObject, err error) {
	blockHash, err := eth.DecDataToBytes(hash)
	if err != nil {
		return resp, err
	}

	snapshot := s.StateProvider.ReadOnlyState()
	defer snapshot.Release()

	// TODO: Reading from the TM block store could take a while, might be more efficient to release
	//       the current snapshot and get a new one after pulling out whatever we need from the TM
	//       block store.
	height, err := query.GetBlockHeightFromHash(s.BlockStore, snapshot, blockHash)
	if err != nil {
		return resp, err
	}
	r, err := s.ReceiptHandlerProvider.ReaderAt(snapshot.Block().Height, snapshot.FeatureEnabled(diademchain.EvmTxReceiptsVersion2Feature, false))
	if err != nil {
		return resp, err
	}
	return query.GetBlockByNumber(s.BlockStore, snapshot, height, full, r)
}

// https://github.com/ethereum/wiki/wiki/JSON-RPC#eth_gettransactionbyhash
func (s *QueryServer) EthGetTransactionByHash(hash eth.Data) (resp eth.JsonTxObject, err error) {
	txHash, err := eth.DecDataToBytes(hash)
	if err != nil {
		return resp, err
	}

	snapshot := s.StateProvider.ReadOnlyState()
	defer snapshot.Release()

	r, err := s.ReceiptHandlerProvider.ReaderAt(snapshot.Block().Height, snapshot.FeatureEnabled(diademchain.EvmTxReceiptsVersion2Feature, false))
	if err != nil {
		return resp, err
	}
	return query.GetTxByHash(snapshot, txHash, r)
}

// https://github.com/ethereum/wiki/wiki/JSON-RPC#eth_gettransactionbyblockHashAndIndex
func (s *QueryServer) EthGetTransactionByBlockHashAndIndex(hash eth.Data, index eth.Quantity) (txObj eth.JsonTxObject, err error) {
	blockHash, err := eth.DecDataToBytes(hash)
	if err != nil {
		return txObj, err
	}

	snapshot := s.StateProvider.ReadOnlyState()
	defer snapshot.Release()

	height, err := query.GetBlockHeightFromHash(s.BlockStore, snapshot, blockHash)
	if err != nil {
		return txObj, err
	}
	txIndex, err := eth.DecQuantityToUint(index)
	if err != nil {
		return txObj, err
	}
	r, err := s.ReceiptHandlerProvider.ReaderAt(snapshot.Block().Height, snapshot.FeatureEnabled(diademchain.EvmTxReceiptsVersion2Feature, false))
	if err != nil {
		return txObj, err
	}
	// TODO: Reading from the TM block store could take a while, might be more efficient to release
	//       the current snapshot and get a new one after pulling out whatever we need from the TM
	//       block store.
	return query.GetTxByBlockAndIndex(s.BlockStore, snapshot, uint64(height), txIndex, r)
}

// https://github.com/ethereum/wiki/wiki/JSON-RPC#eth_gettransactionbyblocknumberandindex
func (s *QueryServer) EthGetTransactionByBlockNumberAndIndex(block eth.BlockHeight, index eth.Quantity) (txObj eth.JsonTxObject, err error) {
	snapshot := s.StateProvider.ReadOnlyState()
	defer snapshot.Release()

	height, err := eth.DecBlockHeight(snapshot.Block().Height, block)
	if err != nil {
		return txObj, err
	}
	txIndex, err := eth.DecQuantityToUint(index)
	if err != nil {
		return txObj, err
	}
	r, err := s.ReceiptHandlerProvider.ReaderAt(snapshot.Block().Height, snapshot.FeatureEnabled(diademchain.EvmTxReceiptsVersion2Feature, false))
	if err != nil {
		return txObj, err
	}
	// TODO: Reading from the TM block store could take a while, might be more efficient to release
	//       the current snapshot and get a new one after pulling out whatever we need from the TM
	//       block store.
	return query.GetTxByBlockAndIndex(s.BlockStore, snapshot, height, txIndex, r)
}

/// https://github.com/ethereum/wiki/wiki/JSON-RPC#eth_getlogs
func (s *QueryServer) EthGetLogs(filter eth.JsonFilter) (resp []eth.JsonLog, err error) {
	ethFilter, err := eth.DecLogFilter(filter)
	if err != nil {
		return resp, err
	}

	snapshot := s.StateProvider.ReadOnlyState()
	defer snapshot.Release()

	r, err := s.ReceiptHandlerProvider.ReaderAt(snapshot.Block().Height, snapshot.FeatureEnabled(diademchain.EvmTxReceiptsVersion2Feature, false))
	if err != nil {
		return resp, err
	}
	// TODO: Reading from the TM block store could take a while, might be more efficient to release
	//       the current snapshot and get a new one after pulling out whatever we need from the TM
	//       block store.
	logs, err := query.QueryChain(s.BlockStore, snapshot, ethFilter, r)
	if err != nil {
		return resp, err
	}
	return eth.EncLogs(logs), err
}

// todo add EthNewBlockFilter EthNewPendingTransactionFilter EthUninstallFilter EthGetFilterChanges and EthGetFilterLogs
// https://github.com/ethereum/wiki/wiki/JSON-RPC#eth_newblockfilter
func (s QueryServer) EthNewBlockFilter() (eth.Quantity, error) {
	state := s.StateProvider.ReadOnlyState()
	return eth.Quantity(s.EthPolls.AddBlockPoll(uint64(state.Block().Height))), nil
}

// https://github.com/ethereum/wiki/wiki/JSON-RPC#eth_newpendingtransactionfilter
func (s QueryServer) EthNewPendingTransactionFilter() (eth.Quantity, error) {
	state := s.StateProvider.ReadOnlyState()
	return eth.Quantity(s.EthPolls.AddTxPoll(uint64(state.Block().Height))), nil
}

// Forget the filter.
// https://github.com/ethereum/wiki/wiki/JSON-RPC#eth_uninstallfilter
func (s *QueryServer) EthUninstallFilter(id eth.Quantity) (bool, error) {
	s.EthPolls.Remove(string(id))
	return true, nil
}

// Get the logs since last poll
// https://github.com/ethereum/wiki/wiki/JSON-RPC#eth_getfilterchanges
func (s *QueryServer) EthGetFilterChanges(id eth.Quantity) (interface{}, error) {
	snapshot := s.StateProvider.ReadOnlyState()
	defer snapshot.Release()
	r, err := s.ReceiptHandlerProvider.ReaderAt(snapshot.Block().Height, snapshot.FeatureEnabled(diademchain.EvmTxReceiptsVersion2Feature, false))
	if err != nil {
		return nil, err
	}

	state := s.StateProvider.ReadOnlyState()
	return s.EthPolls.Poll(s.BlockStore, state, string(id), r)
}

// https://github.com/ethereum/wiki/wiki/JSON-RPC#eth_getfilterlogs
func (s *QueryServer) EthGetFilterLogs(id eth.Quantity) (interface{}, error) {
	state := s.StateProvider.ReadOnlyState()
	snapshot := s.StateProvider.ReadOnlyState()
	defer snapshot.Release()
	r, err := s.ReceiptHandlerProvider.ReaderAt(snapshot.Block().Height, snapshot.FeatureEnabled(diademchain.EvmTxReceiptsVersion2Feature, false))
	if err != nil {
		return nil, err
	}

	if filter, err := s.EthSubscriptions.GetFilter(string(id)); filter != nil || err != nil {
		logs, err := query.QueryChain(s.BlockStore, state, *filter, r)
		if err != nil {
			return nil, err
		}
		return eth.EncLogs(logs), err
	} else {
		return s.EthPolls.AllLogs(s.BlockStore, state, string(id), r)
	}
}

// Sets up new filter for polling
// https://github.com/ethereum/wiki/wiki/JSON-RPC#eth_newfilter
func (s *QueryServer) EthNewFilter(filter eth.JsonFilter) (eth.Quantity, error) {
	state := s.StateProvider.ReadOnlyState()
	ethFilter, err := eth.DecLogFilter(filter)
	if err != nil {
		return "", errors.Wrap(err, "could decode log filter")
	}
	id, err := s.EthPolls.AddLogPoll(ethFilter, uint64(state.Block().Height))
	return eth.Quantity(id), err
}

func (s *QueryServer) EthSubscribe(conn *websocket.Conn, method eth.Data, filter eth.JsonFilter) (eth.Data, error) {
	f, err := eth.DecLogFilter(filter)
	if err != nil {
		return "", errors.Wrapf(err, "decode filter")
	}
	id, err := s.EthSubscriptions.AddSubscription(string(method), f, conn)
	if err != nil {
		return "", errors.Wrapf(err, "add subscription")
	}
	return eth.Data(id), nil
}

func (s *QueryServer) EthUnsubscribe(id eth.Quantity) (unsubscribed bool, err error) {
	s.EthSubscriptions.Remove(string(id))
	return true, nil
}

func (s *QueryServer) EthGetTransactionCount(local eth.Data, block eth.BlockHeight) (eth.Quantity, error) {
	snapshot := s.StateProvider.ReadOnlyState()
	defer snapshot.Release()

	height, err := eth.DecBlockHeight(snapshot.Block().Height, block)
	if err != nil {
		return eth.Quantity("0x0"), err
	}

	if height != uint64(snapshot.Block().Height) {
		return eth.Quantity("0x0"), fmt.Errorf("transaction count only implemted for the latest block %v, block %v requested", snapshot.Block().Height, height)
	}
	address, err := eth.DecDataToAddress(s.ChainID, local)
	if err != nil {
		return eth.Quantity("0x0"), err
	}
	nonce, err := s.Nonce("", address.String())
	if err != nil {
		return eth.Quantity("0x0"), errors.Wrap(err, "requesting transaction count")
	}

	return eth.EncUint(nonce), nil
}

func (s *QueryServer) EthGetBalance(address eth.Data, block eth.BlockHeight) (eth.Quantity, error) {
	return eth.Quantity("0x0"), nil
}

func (s *QueryServer) EthEstimateGas(query eth.JsonTxCallObject) (eth.Quantity, error) {
	return eth.Quantity("0x0"), nil
}

func (s *QueryServer) EthGasPrice() (eth.Quantity, error) {
	return eth.Quantity("0x0"), nil
}

func (s *QueryServer) EthNetVersion() (string, error) {
	hash := sha3.SoliditySHA3(sha3.String(s.ChainID))
	versionBigInt := new(big.Int)
	versionBigInt.SetString(hex.EncodeToString(hash)[0:13], 16)
	return versionBigInt.String(), nil
}

func (s *QueryServer) EthAccounts() ([]eth.Data, error) {
	return []eth.Data{}, nil
}
