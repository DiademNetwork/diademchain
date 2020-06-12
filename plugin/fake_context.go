// +build evm

package plugin

import (
	"context"
	"time"

	diadem "github.com/diademnetwork/go-diadem"
	"github.com/diademnetwork/go-diadem/plugin"
	"github.com/diademnetwork/go-diadem/types"
	"github.com/diademnetwork/diademchain"
	levm "github.com/diademnetwork/diademchain/evm"
	abci "github.com/tendermint/tendermint/abci/types"
	dbm "github.com/tendermint/tendermint/libs/db"
)

// Contract context for tests that need both Go & EVM contracts.
type FakeContextWithEVM struct {
	*plugin.FakeContext
	State                    diademchain.State
	EvmDB                    dbm.DB
	useAccountBalanceManager bool
	caller                   diadem.Address
}

func CreateFakeContextWithEVM(caller, address diadem.Address) *FakeContextWithEVM {
	block := abci.Header{
		ChainID: "chain",
		Height:  int64(34),
		Time:    time.Unix(123456789, 0),
	}
	ctx := plugin.CreateFakeContext(caller, address).WithBlock(
		types.BlockHeader{
			ChainID: block.ChainID,
			Height:  block.Height,
			Time:    block.Time.Unix(),
		},
	)
	state := diademchain.NewStoreState(context.Background(), ctx, block, nil, nil)
	return &FakeContextWithEVM{
		FakeContext: ctx,
		State:       state,
		caller:      caller,
		EvmDB:       dbm.NewMemDB(),
	}
}

func (c *FakeContextWithEVM) WithBlock(header diadem.BlockHeader) *FakeContextWithEVM {
	return &FakeContextWithEVM{
		FakeContext:              c.FakeContext.WithBlock(header),
		State:                    c.State,
		EvmDB:                    c.EvmDB,
		useAccountBalanceManager: c.useAccountBalanceManager,
	}
}

func (c *FakeContextWithEVM) WithSender(caller diadem.Address) *FakeContextWithEVM {
	return &FakeContextWithEVM{
		FakeContext:              c.FakeContext.WithSender(caller),
		State:                    c.State,
		EvmDB:                    c.EvmDB,
		useAccountBalanceManager: c.useAccountBalanceManager,
	}
}

func (c *FakeContextWithEVM) WithAddress(addr diadem.Address) *FakeContextWithEVM {
	return &FakeContextWithEVM{
		FakeContext:              c.FakeContext.WithAddress(addr),
		State:                    c.State,
		EvmDB:                    c.EvmDB,
		useAccountBalanceManager: c.useAccountBalanceManager,
	}
}

func (c *FakeContextWithEVM) WithFeature(name string, value bool) *FakeContextWithEVM {
	c.State.SetFeature(name, value)
	return &FakeContextWithEVM{
		FakeContext:              c.FakeContext,
		State:                    c.State,
		EvmDB:                    c.EvmDB,
		useAccountBalanceManager: c.useAccountBalanceManager,
	}
}

func (c *FakeContextWithEVM) WithAccountBalanceManager(enable bool) *FakeContextWithEVM {
	return &FakeContextWithEVM{
		FakeContext:              c.FakeContext,
		State:                    c.State,
		EvmDB:                    c.EvmDB,
		useAccountBalanceManager: enable,
	}
}

func (c *FakeContextWithEVM) AccountBalanceManager(readOnly bool) levm.AccountBalanceManager {
	ethCoinAddr, err := c.Resolve("ethcoin")
	if err != nil {
		panic(err)
	}
	return NewAccountBalanceManager(c.WithAddress(ethCoinAddr))
}

func (c *FakeContextWithEVM) CallEVM(addr diadem.Address, input []byte, value *diadem.BigUInt) ([]byte, error) {
	var createABM levm.AccountBalanceManagerFactoryFunc
	if c.useAccountBalanceManager {
		createABM = c.AccountBalanceManager
	}
	vm := levm.NewDiademVm(c.State, c.EvmDB, nil, nil, createABM, false)
	return vm.Call(c.ContractAddress(), addr, input, value)
}

func (c *FakeContextWithEVM) StaticCallEVM(addr diadem.Address, input []byte) ([]byte, error) {
	var createABM levm.AccountBalanceManagerFactoryFunc
	if c.useAccountBalanceManager {
		createABM = c.AccountBalanceManager
	}
	vm := levm.NewDiademVm(c.State, c.EvmDB, nil, nil, createABM, false)
	return vm.StaticCall(c.ContractAddress(), addr, input)
}

func (c *FakeContextWithEVM) FeatureEnabled(name string, value bool) bool {
	return c.State.FeatureEnabled(name, value)
}
