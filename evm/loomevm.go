// +build evm

package evm

import (
	"encoding/json"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/types"
	ethvm "github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/ethdb"
	"github.com/gogo/protobuf/proto"
	"github.com/diademnetwork/go-diadem"
	ptypes "github.com/diademnetwork/go-diadem/plugin/types"
	"github.com/diademnetwork/diademchain"
	"github.com/diademnetwork/diademchain/vm"
	"github.com/pkg/errors"
	dbm "github.com/tendermint/tendermint/libs/db"
)

var (
	vmPrefix = []byte("vm")
	rootKey  = []byte("vmroot")
)

type StateDB interface {
	ethvm.StateDB
	Database() state.Database
	Logs() []*types.Log
	Commit(bool) (common.Hash, error)
}

type ethdbLogContext struct {
	blockHeight  int64
	contractAddr diadem.Address
	callerAddr   diadem.Address
}

// TODO: this doesn't need to be exported, rename to diademEvmWithState
type DiademEvm struct {
	*Evm
	db        ethdb.Database
	sdb       StateDB
	diademState diademchain.State
}

// TODO: this doesn't need to be exported, rename to newDiademEvmWithState
func NewDiademEvm(
	diademState diademchain.State,
	evmDB dbm.DB,
	accountBalanceManager AccountBalanceManager,
	logContext *MultiWriterDBLogContext,
	debug bool,
) (*DiademEvm, error) {
	p := new(DiademEvm)
	p.diademState = diademState

	// If EvmDBFeature is enabled, read from and write to evm.db
	// otherwise write to both evm.db and app.db but read from app.db
	var config MultiWriterDBConfig
	var diademEthDB *DiademEthDB
	if diademState.FeatureEnabled(diademchain.EvmDBFeature, false) {
		config = MultiWriterDBConfig{
			Read:  EVM_DB,
			Write: EVM_DB,
		}
	} else {
		diademEthDB = NewDiademEthDB(diademState, MultiWriterDBLogContextToEthDbLogContext(logContext))
		config = MultiWriterDBConfig{
			Read:  DIADEM_ETH_DB,
			Write: ALL_DB,
		}
	}

	p.db = NewMultiWriterDB(evmDB, diademEthDB, logContext, config)

	// Get current EVM Patricia root from IAVL tree (app.db)
	oldRoot := diademState.Get(rootKey)

	var abm *evmAccountBalanceManager
	var err error
	if accountBalanceManager != nil {
		abm = newEVMAccountBalanceManager(accountBalanceManager, diademState.Block().ChainID)
		p.sdb, err = newDiademStateDB(abm, common.BytesToHash(oldRoot), state.NewDatabase(p.db))
	} else {
		p.sdb, err = state.New(common.BytesToHash(oldRoot), state.NewDatabase(p.db))

	}

	if err != nil {
		return nil, err
	}

	p.Evm = NewEvm(p.sdb, diademState, abm, debug)
	return p, nil
}

func (levm DiademEvm) Commit() (common.Hash, error) {
	root, err := levm.sdb.Commit(true)
	if err != nil {
		return root, err
	}
	if err := levm.sdb.Database().TrieDB().Commit(root, false); err != nil {
		return root, err
	}
	if err := levm.db.Put(rootKey, root[:]); err != nil {
		return root, err
	}

	// Track the current root of Patricia tree
	levm.diademState.Set(rootKey, root[:])
	// Save the root of Patricia tree in app.db so that we can rollback evm.db
	levm.diademState.Set(diademchain.EvmDBMapperKey(levm.diademState.Block().Height), root[:])

	return root, err
}

func (levm DiademEvm) RawDump() []byte {
	d := levm.sdb.RawDump()
	output, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		panic(err)
	}
	return output
}

// DiademVm implements the diademchain/vm.VM interface using the EVM.
// TODO: rename to DiademEVM
type DiademVm struct {
	state          diademchain.State
	evmDB          dbm.DB
	receiptHandler diademchain.WriteReceiptHandler
	createABM      AccountBalanceManagerFactoryFunc
	debug          bool
}

func NewDiademVm(
	diademState diademchain.State,
	evmDB dbm.DB,
	eventHandler diademchain.EventHandler,
	receiptHandler diademchain.WriteReceiptHandler,
	createABM AccountBalanceManagerFactoryFunc,
	debug bool,
) vm.VM {
	return &DiademVm{
		state:          diademState,
		evmDB:          evmDB,
		receiptHandler: receiptHandler,
		createABM:      createABM,
		debug:          debug,
	}
}

func (lvm DiademVm) accountBalanceManager(readOnly bool) AccountBalanceManager {
	if lvm.createABM == nil {
		return nil
	}
	return lvm.createABM(readOnly)
}

func (lvm DiademVm) Create(caller diadem.Address, code []byte, value *diadem.BigUInt) ([]byte, diadem.Address, error) {
	logContext := &MultiWriterDBLogContext{
		BlockHeight:  lvm.state.Block().Height,
		ContractAddr: diadem.Address{},
		CallerAddr:   caller,
	}
	levm, err := NewDiademEvm(lvm.state, lvm.evmDB, lvm.accountBalanceManager(false), logContext, lvm.debug)
	if err != nil {
		return nil, diadem.Address{}, err
	}
	bytecode, addr, err := levm.Create(caller, code, value)
	if err == nil {
		_, err = levm.Commit()
	}

	var txHash []byte
	if lvm.receiptHandler != nil {
		var events []*ptypes.EventData
		if err == nil {
			events = lvm.receiptHandler.GetEventsFromLogs(
				levm.sdb.Logs(), lvm.state.Block().Height, caller, addr, code,
			)
		}

		var errSaveReceipt error
		txHash, errSaveReceipt = lvm.receiptHandler.CacheReceipt(lvm.state, caller, addr, events, err)
		if errSaveReceipt != nil {
			err = errors.Wrapf(err, "trouble saving receipt %v", errSaveReceipt)
		}
	}

	response, errMarshal := proto.Marshal(&vm.DeployResponseData{
		TxHash:   txHash,
		Bytecode: bytecode,
	})
	if errMarshal != nil {
		if err == nil {
			return []byte{}, addr, errMarshal
		} else {
			return []byte{}, addr, errors.Wrapf(err, "error marshaling %v", errMarshal)
		}
	}
	return response, addr, err
}

func (lvm DiademVm) Call(caller, addr diadem.Address, input []byte, value *diadem.BigUInt) ([]byte, error) {
	logContext := &MultiWriterDBLogContext{
		BlockHeight:  lvm.state.Block().Height,
		ContractAddr: addr,
		CallerAddr:   caller,
	}
	levm, err := NewDiademEvm(lvm.state, lvm.evmDB, lvm.accountBalanceManager(false), logContext, lvm.debug)
	if err != nil {
		return nil, err
	}
	_, err = levm.Call(caller, addr, input, value)
	if err == nil {
		_, err = levm.Commit()
	}

	var txHash []byte
	if lvm.receiptHandler != nil {
		var events []*ptypes.EventData
		if err == nil {
			events = lvm.receiptHandler.GetEventsFromLogs(
				levm.sdb.Logs(), lvm.state.Block().Height, caller, addr, input,
			)
		}

		var errSaveReceipt error
		txHash, errSaveReceipt = lvm.receiptHandler.CacheReceipt(lvm.state, caller, addr, events, err)
		if errSaveReceipt != nil {
			err = errors.Wrapf(err, "trouble saving receipt %v", errSaveReceipt)
		}
	}
	return txHash, err
}

func (lvm DiademVm) StaticCall(caller, addr diadem.Address, input []byte) ([]byte, error) {
	levm, err := NewDiademEvm(lvm.state, lvm.evmDB, lvm.accountBalanceManager(true), nil, lvm.debug)
	if err != nil {
		return nil, err
	}
	return levm.StaticCall(caller, addr, input)
}

func (lvm DiademVm) GetCode(addr diadem.Address) ([]byte, error) {
	levm, err := NewDiademEvm(lvm.state, lvm.evmDB, nil, nil, lvm.debug)
	if err != nil {
		return nil, err
	}
	return levm.GetCode(addr), nil
}

func MultiWriterDBLogContextToEthDbLogContext(multiWriterLogContext *MultiWriterDBLogContext) *ethdbLogContext {
	var logContext *ethdbLogContext
	if multiWriterLogContext != nil {
		logContext = &ethdbLogContext{
			blockHeight:  multiWriterLogContext.BlockHeight,
			contractAddr: multiWriterLogContext.ContractAddr,
			callerAddr:   multiWriterLogContext.ContractAddr,
		}
	}
	return logContext
}
