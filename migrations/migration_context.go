package migrations

import (
	"github.com/pkg/errors"

	diadem "github.com/diademnetwork/go-diadem"
	"github.com/diademnetwork/go-diadem/plugin/contractpb"
	"github.com/diademnetwork/diademchain"
	genesiscfg "github.com/diademnetwork/diademchain/config/genesis"
	"github.com/diademnetwork/diademchain/core"
	"github.com/diademnetwork/diademchain/plugin"
	registry "github.com/diademnetwork/diademchain/registry/factory"
	"github.com/diademnetwork/diademchain/vm"
)

var (
	ErrLoaderNotFound = errors.New("loader not found")
)

// MigrationContext is available within migration functions, it can be used to deploy new contracts,
// and to call Go & EVM contracts.
type MigrationContext struct {
	manager        *vm.Manager
	createRegistry registry.RegistryFactoryFunc
	caller         diadem.Address
	state          diademchain.State
	codeLoaders    map[string]core.ContractCodeLoader
}

func NewMigrationContext(
	manager *vm.Manager,
	createRegistry registry.RegistryFactoryFunc,
	state diademchain.State,
	caller diadem.Address,
) *MigrationContext {
	return &MigrationContext{
		manager:        manager,
		createRegistry: createRegistry,
		state:          state,
		caller:         caller,
		codeLoaders:    core.GetDefaultCodeLoaders(),
	}
}

// State returns the app state.
func (mc *MigrationContext) State() diademchain.State {
	return mc.state
}

// DeployContract deploys a Go contract and returns its address.
func (mc *MigrationContext) DeployContract(contractCfg *genesiscfg.ContractConfig) (diadem.Address, error) {
	vmType := contractCfg.VMType()
	vm, err := mc.manager.InitVM(vmType, mc.state)
	if err != nil {
		return diadem.Address{}, err
	}

	loader, found := mc.codeLoaders[contractCfg.Format]
	if !found {
		return diadem.Address{}, errors.Wrapf(ErrLoaderNotFound, "contract format: %s", contractCfg.Format)
	}
	initCode, err := loader.LoadContractCode(
		contractCfg.Location,
		contractCfg.Init,
	)
	if err != nil {
		return diadem.Address{}, err
	}
	_, addr, err := vm.Create(mc.caller, initCode, diadem.NewBigUIntFromInt(0))
	if err != nil {
		return diadem.Address{}, err
	}

	reg := mc.createRegistry(mc.state)
	if err := reg.Register(contractCfg.Name, addr, mc.caller); err != nil {

		return diadem.Address{}, err
	}

	return addr, nil
}

// ContractContext returns the context for the Go contract with the given name.
// The returned context can be used to call the named contract.
func (mc *MigrationContext) ContractContext(contractName string) (contractpb.Context, error) {
	reg := mc.createRegistry(mc.state)
	contractAddr, err := reg.Resolve(contractName)
	if err != nil {
		return nil, err
	}

	vm, err := mc.manager.InitVM(vm.VMType_PLUGIN, mc.state)
	if err != nil {
		return nil, err
	}

	pluginVM := vm.(*plugin.PluginVM) // Ugh
	readOnly := false
	return contractpb.WrapPluginContext(pluginVM.CreateContractContext(mc.caller, contractAddr, readOnly)), nil
}
