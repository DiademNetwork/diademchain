// +build evm

package gateway

import (
	"io/ioutil"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/gogo/protobuf/proto"
	"github.com/diademnetwork/go-diadem"
	contract "github.com/diademnetwork/go-diadem/plugin/contractpb"
	"github.com/diademnetwork/go-diadem/types"
	"github.com/diademnetwork/diademchain/builtin/plugins/address_mapper"
	levm "github.com/diademnetwork/diademchain/evm"
	"github.com/diademnetwork/diademchain/plugin"
	"github.com/pkg/errors"
)

// Returns all unclaimed tokens for an account
func unclaimedTokensByOwner(ctx contract.StaticContext, ownerAddr diadem.Address) ([]*UnclaimedToken, error) {
	result := []*UnclaimedToken{}
	ownerKey := unclaimedTokensRangePrefix(ownerAddr)
	for _, entry := range ctx.Range(ownerKey) {
		var unclaimedToken UnclaimedToken
		if err := proto.Unmarshal(entry.Value, &unclaimedToken); err != nil {
			return nil, errors.Wrap(err, ErrFailedToReclaimToken.Error())
		}
		result = append(result, &unclaimedToken)
	}
	return result, nil
}

// Returns all unclaimed tokens for a token contract
func unclaimedTokenDepositorsByContract(ctx contract.StaticContext, tokenAddr diadem.Address) ([]diadem.Address, error) {
	result := []diadem.Address{}
	contractKey := unclaimedTokenDepositorsRangePrefix(tokenAddr)
	for _, entry := range ctx.Range(contractKey) {
		var addr types.Address
		if err := proto.Unmarshal(entry.Value, &addr); err != nil {
			return nil, errors.Wrap(err, ErrFailedToReclaimToken.Error())
		}
		result = append(result, diadem.UnmarshalAddressPB(&addr))
	}
	return result, nil
}

func genERC721Deposits(tokenAddr, owner diadem.Address, blocks []uint64, values [][]int64) []*MainnetEvent {
	if len(values) > 0 && len(values) != len(blocks) {
		panic("insufficient number of values")
	}
	result := []*MainnetEvent{}
	for i, b := range blocks {
		numTokens := 5
		if len(values) > 0 {
			numTokens = len(values[i])
		}
		for j := 0; j < numTokens; j++ {
			tokenID := diadem.NewBigUIntFromInt(int64(j + 1))
			if len(values) > 0 {
				tokenID = diadem.NewBigUIntFromInt(values[i][j])
			}
			result = append(result, &MainnetEvent{
				EthBlock: b,
				Payload: &MainnetDepositEvent{
					Deposit: &MainnetTokenDeposited{
						TokenKind:     TokenKind_ERC721,
						TokenContract: tokenAddr.MarshalPB(),
						TokenOwner:    owner.MarshalPB(),
						Value: &types.BigUInt{
							Value: *tokenID,
						},
					},
				},
			})
		}
	}
	return result
}

func genERC20Deposits(tokenAddr, owner diadem.Address, blocks []uint64, values []int64) []*MainnetEvent {
	if len(values) != len(blocks) {
		panic("insufficient number of values")
	}
	result := []*MainnetEvent{}
	for i, b := range blocks {
		result = append(result, &MainnetEvent{
			EthBlock: b,
			Payload: &MainnetDepositEvent{
				Deposit: &MainnetTokenDeposited{
					TokenKind:     TokenKind_ERC20,
					TokenContract: tokenAddr.MarshalPB(),
					TokenOwner:    owner.MarshalPB(),
					Value: &types.BigUInt{
						Value: *diadem.NewBigUIntFromInt(values[i]),
					},
				},
			},
		})
	}
	return result
}

type testAddressMapperContract struct {
	Contract *address_mapper.AddressMapper
	Address  diadem.Address
}

func (am *testAddressMapperContract) AddIdentityMapping(ctx *plugin.FakeContextWithEVM, from, to diadem.Address, sig []byte) error {
	return am.Contract.AddIdentityMapping(
		contract.WrapPluginContext(ctx.WithAddress(am.Address)),
		&address_mapper.AddIdentityMappingRequest{
			From:      from.MarshalPB(),
			To:        to.MarshalPB(),
			Signature: sig,
		})
}

func deployAddressMapperContract(ctx *plugin.FakeContextWithEVM) (*testAddressMapperContract, error) {
	amContract := &address_mapper.AddressMapper{}
	amAddr := ctx.CreateContract(contract.MakePluginContract(amContract))
	amCtx := contract.WrapPluginContext(ctx.WithAddress(amAddr))

	err := amContract.Init(amCtx, &address_mapper.InitRequest{})
	if err != nil {
		return nil, err
	}
	return &testAddressMapperContract{
		Contract: amContract,
		Address:  amAddr,
	}, nil
}

type testGatewayContract struct {
	Contract *Gateway
	Address  diadem.Address
}

func (gc *testGatewayContract) ContractCtx(ctx *plugin.FakeContextWithEVM) contract.Context {
	return contract.WrapPluginContext(ctx.WithAddress(gc.Address))
}

func (gc *testGatewayContract) AddContractMapping(ctx *plugin.FakeContextWithEVM, foreignContractAddr, localContractAddr diadem.Address) error {
	contractCtx := gc.ContractCtx(ctx)
	err := contractCtx.Set(contractAddrMappingKey(foreignContractAddr), &ContractAddressMapping{
		From: foreignContractAddr.MarshalPB(),
		To:   localContractAddr.MarshalPB(),
	})
	if err != nil {
		return err
	}
	err = contractCtx.Set(contractAddrMappingKey(localContractAddr), &ContractAddressMapping{
		From: localContractAddr.MarshalPB(),
		To:   foreignContractAddr.MarshalPB(),
	})
	if err != nil {
		return err
	}
	return nil
}

func deployGatewayContract(ctx *plugin.FakeContextWithEVM, genesis *InitRequest) (*testGatewayContract, error) {
	gwContract := &Gateway{}
	gwAddr := ctx.CreateContract(contract.MakePluginContract(gwContract))
	gwCtx := contract.WrapPluginContext(ctx.WithAddress(gwAddr))

	err := gwContract.Init(gwCtx, genesis)
	return &testGatewayContract{
		Contract: gwContract,
		Address:  gwAddr,
	}, err
}

func deployTokenContract(ctx *plugin.FakeContextWithEVM, filename string, gateway, caller diadem.Address) (diadem.Address, error) {
	contractAddr := diadem.Address{}
	hexByteCode, err := ioutil.ReadFile("../testdata/" + filename + ".bin")
	if err != nil {
		return contractAddr, err
	}
	abiBytes, err := ioutil.ReadFile("../testdata/" + filename + ".abi")
	if err != nil {
		return contractAddr, err
	}
	contractABI, err := abi.JSON(strings.NewReader(string(abiBytes)))
	if err != nil {
		return contractAddr, err
	}
	byteCode := common.FromHex(string(hexByteCode))
	// append constructor args to bytecode
	input, err := contractABI.Pack("", common.BytesToAddress(gateway.Local))
	if err != nil {
		return contractAddr, err
	}
	byteCode = append(byteCode, input...)

	vm := levm.NewDiademVm(ctx.State, ctx.EvmDB, nil, nil, nil, false)
	_, contractAddr, err = vm.Create(caller, byteCode, diadem.NewBigUIntFromInt(0))
	if err != nil {
		return contractAddr, err
	}
	ctx.RegisterContract("", contractAddr, caller)
	return contractAddr, nil
}
