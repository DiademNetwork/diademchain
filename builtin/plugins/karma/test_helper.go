package karma

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/gogo/protobuf/proto"
	"github.com/diademnetwork/go-diadem"
	ctypes "github.com/diademnetwork/go-diadem/builtin/types/coin"
	ktypes "github.com/diademnetwork/go-diadem/builtin/types/karma"
	"github.com/diademnetwork/go-diadem/plugin/contractpb"
	"github.com/diademnetwork/diademchain"
	"github.com/diademnetwork/diademchain/builtin/plugins/coin"
	"github.com/diademnetwork/diademchain/log"
	"github.com/diademnetwork/diademchain/plugin"
	"github.com/diademnetwork/diademchain/registry"
	"github.com/diademnetwork/diademchain/registry/factory"
	"github.com/diademnetwork/diademchain/store"
	"github.com/diademnetwork/diademchain/vm"
	"github.com/stretchr/testify/require"
	abci "github.com/tendermint/tendermint/abci/types"
	"github.com/tendermint/tendermint/libs/db"
)

// TODO: This duplicates a lot of the contract loading & deployment code, it should just use the
//       mock context, or if that's not sufficient the contract loading code may need to be
//       refactored to make it possible to eliminate these helpers.

func MockStateWithKarmaAndCoinT(t *testing.T, karmaInit *ktypes.KarmaInitRequest, coinInit *ctypes.InitRequest) (diademchain.State, registry.Registry, vm.VM) {
	appDb := db.NewMemDB()
	state, reg, pluginVm, err := MockStateWithKarmaAndCoin(karmaInit, coinInit, appDb)
	require.NoError(t, err)
	return state, reg, pluginVm
}

func MockStateWithKarmaAndCoinB(b *testing.B, karmaInit *ktypes.KarmaInitRequest, coinInit *ctypes.InitRequest, appDbName string) (diademchain.State, registry.Registry, vm.VM) {
	appDb, err := db.NewGoLevelDB(appDbName, ".")
	state, reg, pluginVm, err := MockStateWithKarmaAndCoin(karmaInit, coinInit, appDb)
	require.NoError(b, err)
	return state, reg, pluginVm
}

func MockStateWithKarmaAndCoin(karmaInit *ktypes.KarmaInitRequest, coinInit *ctypes.InitRequest, appDb db.DB) (diademchain.State, registry.Registry, vm.VM, error) {
	appStore, err := store.NewIAVLStore(appDb, 0, 0)
	header := abci.Header{}
	header.Height = int64(1)
	state := diademchain.NewStoreState(context.Background(), appStore, header, nil, nil)

	vmManager := vm.NewManager()
	createRegistry, err := factory.NewRegistryFactory(factory.RegistryV2)
	reg := createRegistry(state)
	if err != nil {
		return nil, nil, nil, err
	}
	loader := plugin.NewStaticLoader(Contract, coin.Contract)
	vmManager.Register(vm.VMType_PLUGIN, func(state diademchain.State) (vm.VM, error) {
		return plugin.NewPluginVM(loader, state, db.NewMemDB(), reg, nil, log.Default, nil, nil, nil), nil
	})
	pluginVm, err := vmManager.InitVM(vm.VMType_PLUGIN, state)
	if err != nil {
		return nil, nil, nil, err
	}

	if karmaInit != nil {
		karmaCode, err := json.Marshal(karmaInit)
		if err != nil {
			return nil, nil, nil, err
		}
		karmaInitCode, err := LoadContractCode("karma:1.0.0", karmaCode)
		if err != nil {
			return nil, nil, nil, err
		}
		callerAddr := plugin.CreateAddress(diadem.RootAddress("chain"), uint64(0))
		_, karmaAddr, err := pluginVm.Create(callerAddr, karmaInitCode, diadem.NewBigUIntFromInt(0))
		if err != nil {
			return nil, nil, nil, err
		}
		err = reg.Register("karma", karmaAddr, karmaAddr)
		if err != nil {
			return nil, nil, nil, err
		}
	}

	if coinInit != nil {
		coinCode, err := json.Marshal(coinInit)
		if err != nil {
			return nil, nil, nil, err
		}
		coinInitCode, err := LoadContractCode("coin:1.0.0", coinCode)
		if err != nil {
			return nil, nil, nil, err
		}
		callerAddr := plugin.CreateAddress(diadem.RootAddress("chain"), uint64(1))
		_, coinAddr, err := pluginVm.Create(callerAddr, coinInitCode, diadem.NewBigUIntFromInt(0))
		if err != nil {
			return nil, nil, nil, err
		}
		err = reg.Register("coin", coinAddr, coinAddr)
		if err != nil {
			return nil, nil, nil, err
		}
	}
	return state, reg, pluginVm, nil
}

// copied from PluginCodeLoader.LoadContractCode maybe move PluginCodeLoader to separate package
func LoadContractCode(location string, init json.RawMessage) ([]byte, error) {
	body, err := init.MarshalJSON()
	if err != nil {
		return nil, err
	}

	req := &plugin.Request{
		ContentType: plugin.EncodingType_JSON,
		Body:        body,
	}

	input, err := proto.Marshal(req)
	if err != nil {
		return nil, err
	}

	pluginCode := &plugin.PluginCode{
		Name:  location,
		Input: input,
	}
	return proto.Marshal(pluginCode)
}

func MockDeployEvmContract(t *testing.T, karamContractCtx contractpb.Context, owner diadem.Address, nonce uint64) diadem.Address {
	contractAddr := plugin.CreateAddress(owner, nonce)
	require.NoError(t, AddOwnedContract(karamContractCtx, owner, contractAddr))
	return contractAddr
}
