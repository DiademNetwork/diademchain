package plugin

import (
	"bytes"
	"encoding/binary"
	"time"

	"github.com/gogo/protobuf/proto"
	"github.com/diademnetwork/go-diadem/plugin/contractpb"
	"github.com/diademnetwork/go-diadem/plugin/types"
	ltypes "github.com/diademnetwork/go-diadem/types"
	dbm "github.com/tendermint/tendermint/libs/db"
	"golang.org/x/crypto/sha3"

	"github.com/diademnetwork/go-diadem"
	lp "github.com/diademnetwork/go-diadem/plugin"
	"github.com/diademnetwork/go-diadem/util"
	"github.com/diademnetwork/diademchain"
	"github.com/diademnetwork/diademchain/auth"
	levm "github.com/diademnetwork/diademchain/evm"
	"github.com/diademnetwork/diademchain/log"
	"github.com/diademnetwork/diademchain/registry"
	"github.com/diademnetwork/diademchain/vm"
	"github.com/pkg/errors"
)

type (
	Request    = lp.Request
	Response   = lp.Response
	PluginCode = lp.Code
)

var (
	EncodingType_JSON = lp.EncodingType_JSON
)

type PluginVM struct {
	Loader       Loader
	State        diademchain.State
	EvmDB        dbm.DB
	Registry     registry.Registry
	EventHandler diademchain.EventHandler
	logger       *diadem.Logger
	// If this is nil the EVM won't have access to any account balances.
	newABMFactory NewAccountBalanceManagerFactoryFunc
	receiptWriter diademchain.WriteReceiptHandler
	receiptReader diademchain.ReadReceiptHandler
}

func NewPluginVM(
	loader Loader,
	state diademchain.State,
	evmDB dbm.DB,
	registry registry.Registry,
	eventHandler diademchain.EventHandler,
	logger *diadem.Logger,
	newABMFactory NewAccountBalanceManagerFactoryFunc,
	receiptWriter diademchain.WriteReceiptHandler,
	receiptReader diademchain.ReadReceiptHandler,
) *PluginVM {
	return &PluginVM{
		Loader:        loader,
		State:         state,
		EvmDB:         evmDB,
		Registry:      registry,
		EventHandler:  eventHandler,
		logger:        logger,
		newABMFactory: newABMFactory,
		receiptWriter: receiptWriter,
		receiptReader: receiptReader,
	}
}

var _ vm.VM = &PluginVM{}

func (vm *PluginVM) CreateContractContext(
	caller,
	addr diadem.Address,
	readOnly bool,
) *contractContext {
	return &contractContext{
		caller:       caller,
		address:      addr,
		State:        vm.State.WithPrefix(diadem.DataPrefix(addr)),
		EvmDB:        vm.EvmDB,
		VM:           vm,
		Registry:     vm.Registry,
		eventHandler: vm.EventHandler,
		readOnly:     readOnly,
		req:          &Request{},
		logger:       vm.logger,
	}
}

func (vm *PluginVM) run(
	caller,
	addr diadem.Address,
	code,
	input []byte,
	readOnly bool,
) ([]byte, error) {
	var pluginCode PluginCode
	err := proto.Unmarshal(code, &pluginCode)
	if err != nil {
		return nil, err
	}

	contract, err := vm.Loader.LoadContract(pluginCode.Name, vm.State.Block().Height)
	if err != nil {
		return nil, err
	}

	isInit := len(input) == 0
	if isInit {
		input = pluginCode.Input
	}

	req := &Request{}
	err = proto.Unmarshal(input, req)
	if err != nil {
		return nil, err
	}

	contractCtx := vm.CreateContractContext(caller, addr, readOnly)
	contractCtx.pluginName = pluginCode.Name
	contractCtx.req = req

	var res *Response
	if isInit {
		err = contract.Init(contractCtx, req)
		if err != nil {
			return nil, err
		}
		return proto.Marshal(&PluginCode{
			Name: pluginCode.Name,
		})
	}

	if readOnly {
		res, err = contract.StaticCall(contractCtx, req)
	} else {
		res, err = contract.Call(contractCtx, req)
	}

	if err != nil {
		return nil, err
	}

	return proto.Marshal(res)
}

func CreateAddress(parent diadem.Address, nonce uint64) diadem.Address {
	var nonceBuf bytes.Buffer
	binary.Write(&nonceBuf, binary.BigEndian, nonce)
	data := util.PrefixKey(parent.Bytes(), nonceBuf.Bytes())
	hash := sha3.Sum256(data)
	return diadem.Address{
		ChainID: parent.ChainID,
		Local:   hash[12:],
	}
}

func (vm *PluginVM) Create(caller diadem.Address, code []byte, value *diadem.BigUInt) ([]byte, diadem.Address, error) {
	nonce := auth.Nonce(vm.State, caller)
	contractAddr := CreateAddress(caller, nonce)

	ret, err := vm.run(caller, contractAddr, code, nil, false)
	if err != nil {
		return nil, contractAddr, err
	}

	vm.State.Set(diadem.TextKey(contractAddr), ret)
	return ret, contractAddr, nil
}

func (vm *PluginVM) Call(caller, addr diadem.Address, input []byte, value *diadem.BigUInt) ([]byte, error) {
	if len(input) == 0 {
		return nil, errors.New("input is empty")
	}
	code := vm.State.Get(diadem.TextKey(addr))
	return vm.run(caller, addr, code, input, false)
}

func (vm *PluginVM) StaticCall(caller, addr diadem.Address, input []byte) ([]byte, error) {
	if len(input) == 0 {
		return nil, errors.New("input is empty")
	}
	code := vm.State.Get(diadem.TextKey(addr))
	return vm.run(caller, addr, code, input, true)
}

func (vm *PluginVM) CallEVM(caller, addr diadem.Address, input []byte, value *diadem.BigUInt) ([]byte, error) {
	var createABM levm.AccountBalanceManagerFactoryFunc
	var err error
	if vm.newABMFactory != nil {
		createABM, err = vm.newABMFactory(vm)
		if err != nil {
			return nil, err
		}
	}
	evm := levm.NewDiademVm(vm.State, vm.EvmDB, vm.EventHandler, vm.receiptWriter, createABM, false)
	return evm.Call(caller, addr, input, value)
}

func (vm *PluginVM) StaticCallEVM(caller, addr diadem.Address, input []byte) ([]byte, error) {
	var createABM levm.AccountBalanceManagerFactoryFunc
	var err error
	if vm.newABMFactory != nil {
		createABM, err = vm.newABMFactory(vm)
		if err != nil {
			return nil, err
		}
	}
	evm := levm.NewDiademVm(vm.State, vm.EvmDB, vm.EventHandler, vm.receiptWriter, createABM, false)
	return evm.StaticCall(caller, addr, input)
}

func (vm *PluginVM) GetCode(addr diadem.Address) ([]byte, error) {
	return []byte{}, nil
}

// Implements plugin.Context interface (go-diadem/plugin/contract.go)
type contractContext struct {
	caller  diadem.Address
	address diadem.Address
	diademchain.State
	EvmDB dbm.DB
	VM    *PluginVM
	registry.Registry
	eventHandler diademchain.EventHandler
	readOnly     bool
	pluginName   string
	logger       *diadem.Logger
	req          *Request
}

var _ lp.Context = &contractContext{}

func (c *contractContext) Call(addr diadem.Address, input []byte) ([]byte, error) {
	return c.VM.Call(c.address, addr, input, diadem.NewBigUIntFromInt(0))
}

func (c *contractContext) CallEVM(addr diadem.Address, input []byte, value *diadem.BigUInt) ([]byte, error) {
	return c.VM.CallEVM(c.address, addr, input, value)
}

func (c *contractContext) StaticCall(addr diadem.Address, input []byte) ([]byte, error) {
	return c.VM.StaticCall(c.address, addr, input)
}

func (c *contractContext) StaticCallEVM(addr diadem.Address, input []byte) ([]byte, error) {
	return c.VM.StaticCallEVM(c.address, addr, input)
}

func (c *contractContext) Resolve(name string) (diadem.Address, error) {
	return c.Registry.Resolve(name)
}

func (c *contractContext) Message() lp.Message {
	return lp.Message{
		Sender: c.caller,
	}
}

func (c *contractContext) FeatureEnabled(name string, defaultVal bool) bool {
	return c.VM.State.FeatureEnabled(name, defaultVal)
}

func (c *contractContext) Validators() []*ltypes.Validator {
	return c.VM.State.Validators()
}

//TODO don't like how we have to check 3 places, need to clean this up
func (c *contractContext) GetEvmTxReceipt(hash []byte) (types.EvmTxReceipt, error) {
	r, err := c.VM.receiptReader.GetReceipt(c.VM.State, hash)
	if err != nil || len(r.TxHash) == 0 {
		r, err = c.VM.receiptReader.GetPendingReceipt(hash)
		if err != nil || len(r.TxHash) == 0 {
			//[MGC] I made this function return a pointer, its more clear wether or not you got data back
			r2 := c.VM.receiptReader.GetCurrentReceipt()
			if r2 != nil {
				return *r2, nil
			}
			return r, err
		}
	}
	return r, err
}

func (c *contractContext) ContractAddress() diadem.Address {
	return c.address
}

func (c *contractContext) Now() time.Time {
	return time.Unix(c.State.Block().Time, 0)
}

func (c *contractContext) Emit(event []byte) {
	c.EmitTopics(event)
}

func (c *contractContext) EmitTopics(event []byte, topics ...string) {
	log.Debug("emitting event", "bytes", event)
	if c.readOnly {
		return
	}
	data := types.EventData{
		Topics:          topics,
		Caller:          c.caller.MarshalPB(),
		Address:         c.address.MarshalPB(),
		PluginName:      c.pluginName,
		EncodedBody:     event,
		OriginalRequest: c.req.Body,
	}
	height := uint64(c.State.Block().Height)
	c.eventHandler.Post(height, &data)
}

func (c *contractContext) ContractRecord(contractAddr diadem.Address) (*lp.ContractRecord, error) {
	rec, err := c.Registry.GetRecord(contractAddr)
	if err != nil {
		return nil, err
	}
	return &lp.ContractRecord{
		ContractName:    rec.Name,
		ContractAddress: diadem.UnmarshalAddressPB(rec.Address),
		CreatorAddress:  diadem.UnmarshalAddressPB(rec.Owner),
	}, nil
}

// NewInternalContractContext creates an internal Go contract context.
func NewInternalContractContext(contractName string, pluginVM *PluginVM) (contractpb.Context, error) {
	caller := diadem.RootAddress(pluginVM.State.Block().ChainID)
	contractAddr, err := pluginVM.Registry.Resolve(contractName)
	if err != nil {
		return nil, err
	}
	readOnly := false
	return contractpb.WrapPluginContext(pluginVM.CreateContractContext(caller, contractAddr, readOnly)), nil
}
