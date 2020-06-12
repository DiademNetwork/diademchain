package karma

import (
	"github.com/diademnetwork/go-diadem"
	"github.com/diademnetwork/go-diadem/common"
	"github.com/diademnetwork/go-diadem/plugin"
	"github.com/diademnetwork/diademchain/vm"

	"github.com/diademnetwork/diademchain"
	"github.com/diademnetwork/diademchain/registry"
)

type FakeStateContext struct {
	plugin.FakeContext
	state    diademchain.State
	registry registry.Registry
	VM       vm.VM
}

func CreateFakeStateContext(state diademchain.State, reg registry.Registry, caller, address diadem.Address, pluginVm vm.VM) *FakeStateContext {
	fakeContext := plugin.CreateFakeContext(caller, address)
	return &FakeStateContext{
		FakeContext: *fakeContext,
		state:       state.WithPrefix(diadem.DataPrefix(address)),
		registry:    reg,
		VM:          pluginVm,
	}
}

func (c *FakeStateContext) Range(prefix []byte) plugin.RangeData {
	return c.state.Range(prefix)
}

func (c *FakeStateContext) Get(key []byte) []byte {
	return c.state.Get(key)
}

func (c *FakeStateContext) Has(key []byte) bool {
	return c.state.Has(key)
}

func (c *FakeStateContext) Set(key []byte, value []byte) {
	c.state.Set(key, value)
}

func (c *FakeStateContext) Delete(key []byte) {
	c.state.Delete(key)
}

func (c *FakeStateContext) Resolve(name string) (diadem.Address, error) {
	return c.registry.Resolve(name)
}

func (c *FakeStateContext) Call(addr diadem.Address, input []byte) ([]byte, error) {
	return c.VM.Call(c.FakeContext.ContractAddress(), addr, input, common.BigZero())
}
