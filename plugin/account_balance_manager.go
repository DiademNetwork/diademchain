package plugin

import (
	"github.com/diademnetwork/go-diadem"
	"github.com/diademnetwork/go-diadem/plugin"
	contract "github.com/diademnetwork/go-diadem/plugin/contractpb"
	"github.com/diademnetwork/diademchain/builtin/plugins/ethcoin"
	"github.com/diademnetwork/diademchain/evm"
)

// AccountBalanceManager implements the evm.AccountBalanceManager interface using the built-in
// ethcoin contract.
type AccountBalanceManager struct {
	// ethcoin contract context
	ctx  contract.Context
	sctx contract.StaticContext
}

func NewAccountBalanceManager(ctx plugin.Context) *AccountBalanceManager {
	return &AccountBalanceManager{
		ctx:  contract.WrapPluginContext(ctx),
		sctx: contract.WrapPluginStaticContext(ctx),
	}
}

func (m *AccountBalanceManager) GetBalance(addr diadem.Address) (*diadem.BigUInt, error) {
	return ethcoin.BalanceOf(m.sctx, addr)
}

func (m *AccountBalanceManager) AddBalance(addr diadem.Address, amount *diadem.BigUInt) error {
	return ethcoin.AddBalance(m.ctx, addr, amount)
}

func (m *AccountBalanceManager) SubBalance(addr diadem.Address, amount *diadem.BigUInt) error {
	return ethcoin.SubBalance(m.ctx, addr, amount)
}

func (m *AccountBalanceManager) SetBalance(addr diadem.Address, amount *diadem.BigUInt) error {
	return ethcoin.SetBalance(m.ctx, addr, amount)
}

func (m *AccountBalanceManager) Transfer(from, to diadem.Address, amount *diadem.BigUInt) error {
	return ethcoin.Transfer(m.ctx, from, to, amount)
}

type NewAccountBalanceManagerFactoryFunc func(*PluginVM) (evm.AccountBalanceManagerFactoryFunc, error)

func NewAccountBalanceManagerFactory(pvm *PluginVM) (evm.AccountBalanceManagerFactoryFunc, error) {
	ethCoinAddr, err := pvm.Registry.Resolve("ethcoin")
	if err != nil {
		return nil, err
	}
	return func(readOnly bool) evm.AccountBalanceManager {
		caller := diadem.RootAddress(pvm.State.Block().ChainID)
		ctx := pvm.CreateContractContext(caller, ethCoinAddr, readOnly)
		return NewAccountBalanceManager(ctx)
	}, nil
}
