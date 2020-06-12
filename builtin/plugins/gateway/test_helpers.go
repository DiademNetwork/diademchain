// +build evm

package gateway

import (
	"io/ioutil"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	diadem "github.com/diademnetwork/go-diadem"
	contract "github.com/diademnetwork/go-diadem/plugin/contractpb"
	"github.com/diademnetwork/go-diadem/types"
	"github.com/diademnetwork/diademchain/builtin/plugins/address_mapper"
	"github.com/diademnetwork/diademchain/builtin/plugins/coin"
	"github.com/diademnetwork/diademchain/builtin/plugins/ethcoin"
	levm "github.com/diademnetwork/diademchain/evm"
	"github.com/diademnetwork/diademchain/plugin"
)

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
						TokenID:       &types.BigUInt{Value: *tokenID},
					},
				},
			})
		}
	}
	return result
}

func genDiademCoinDeposits(tokenAddr, owner diadem.Address, blocks []uint64, values []int64) []*MainnetEvent {
	if len(values) != len(blocks) {
		panic("insufficient number of values")
	}
	result := []*MainnetEvent{}
	for i, b := range blocks {
		result = append(result, &MainnetEvent{
			EthBlock: b,
			Payload: &MainnetDepositEvent{
				Deposit: &MainnetTokenDeposited{
					TokenKind:     TokenKind_DiademCoin,
					TokenContract: tokenAddr.MarshalPB(),
					TokenOwner:    owner.MarshalPB(),
					TokenAmount:   &types.BigUInt{Value: *diadem.NewBigUIntFromInt(values[i])},
				},
			},
		})
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
					TokenAmount:   &types.BigUInt{Value: *diadem.NewBigUIntFromInt(values[i])},
				},
			},
		})
	}
	return result
}

type erc721xToken struct {
	ID     int64
	Amount int64
}

// Returns a list of ERC721X deposit events, and the total amount per token ID.
func genERC721XDeposits(
	tokenAddr, owner diadem.Address, blocks []uint64, tokens [][]*erc721xToken,
) ([]*MainnetEvent, []*erc721xToken) {
	totals := map[int64]int64{}
	result := []*MainnetEvent{}
	for i, b := range blocks {
		for _, token := range tokens[i] {
			result = append(result, &MainnetEvent{
				EthBlock: b,
				Payload: &MainnetDepositEvent{
					Deposit: &MainnetTokenDeposited{
						TokenKind:     TokenKind_ERC721X,
						TokenContract: tokenAddr.MarshalPB(),
						TokenOwner:    owner.MarshalPB(),
						TokenID:       &types.BigUInt{Value: *diadem.NewBigUIntFromInt(token.ID)},
						TokenAmount:   &types.BigUInt{Value: *diadem.NewBigUIntFromInt(token.Amount)},
					},
				},
			})
			totals[token.ID] = totals[token.ID] + token.Amount
		}
	}

	tokenTotals := []*erc721xToken{}
	for k, v := range totals {
		tokenTotals = append(tokenTotals, &erc721xToken{
			ID:     k,
			Amount: v,
		})
	}
	return result, tokenTotals
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

func deployGatewayContract(ctx *plugin.FakeContextWithEVM, genesis *InitRequest, diademcoinTG bool) (*testGatewayContract, error) {
	gwContract := &Gateway{
		diademCoinTG: diademcoinTG,
	}
	gwAddr := ctx.CreateContract(contract.MakePluginContract(gwContract))
	gwCtx := contract.WrapPluginContext(ctx.WithAddress(gwAddr))

	err := gwContract.Init(gwCtx, genesis)
	return &testGatewayContract{
		Contract: gwContract,
		Address:  gwAddr,
	}, err
}

type testETHContract struct {
	Contract *ethcoin.ETHCoin
	Address  diadem.Address
}

func deployETHContract(ctx *plugin.FakeContextWithEVM) (*testETHContract, error) {
	ethContract := &ethcoin.ETHCoin{}
	contractAddr := ctx.CreateContract(contract.MakePluginContract(ethContract))
	contractCtx := contract.WrapPluginContext(ctx.WithAddress(contractAddr))

	err := ethContract.Init(contractCtx, &ethcoin.InitRequest{})
	return &testETHContract{
		Contract: ethContract,
		Address:  contractAddr,
	}, err
}

func (ec *testETHContract) ContractCtx(ctx *plugin.FakeContextWithEVM) contract.Context {
	return contract.WrapPluginContext(ctx.WithAddress(ec.Address))
}

func (ec *testETHContract) mintToGateway(ctx *plugin.FakeContextWithEVM, amount *big.Int) error {
	return ec.Contract.MintToGateway(ec.ContractCtx(ctx), &ethcoin.MintToGatewayRequest{
		Amount: &types.BigUInt{Value: *diadem.NewBigUInt(amount)},
	})
}

func (ec *testETHContract) approve(ctx *plugin.FakeContextWithEVM, spender diadem.Address, amount *big.Int) error {
	return ec.Contract.Approve(ec.ContractCtx(ctx), &ethcoin.ApproveRequest{
		Spender: spender.MarshalPB(),
		Amount:  &types.BigUInt{Value: *diadem.NewBigUInt(amount)},
	})
}

func (ec *testETHContract) transfer(ctx *plugin.FakeContextWithEVM, to diadem.Address, amount *big.Int) error {
	return ec.Contract.Transfer(ec.ContractCtx(ctx), &ethcoin.TransferRequest{
		To:     to.MarshalPB(),
		Amount: &types.BigUInt{Value: *diadem.NewBigUInt(amount)},
	})
}

type testDiademCoinContract struct {
	Contract *coin.Coin
	Address  diadem.Address
}

func deployDiademCoinContract(ctx *plugin.FakeContextWithEVM) (*testDiademCoinContract, error) {
	coinContract := &coin.Coin{}
	contractAddr := ctx.CreateContract(contract.MakePluginContract(coinContract))
	contractCtx := contract.WrapPluginContext(ctx.WithAddress(contractAddr))
	err := coinContract.Init(contractCtx, &coin.InitRequest{})
	return &testDiademCoinContract{
		Contract: coinContract,
		Address:  contractAddr,
	}, err
}

func (ec *testDiademCoinContract) ContractCtx(ctx *plugin.FakeContextWithEVM) contract.Context {
	return contract.WrapPluginContext(ctx.WithAddress(ec.Address))
}

func (ec *testDiademCoinContract) mintToGateway(ctx *plugin.FakeContextWithEVM, amount *big.Int) error {
	return ec.Contract.MintToGateway(ec.ContractCtx(ctx), &coin.MintToGatewayRequest{
		Amount: &types.BigUInt{Value: *diadem.NewBigUInt(amount)},
	})
}

func (ec *testDiademCoinContract) approve(ctx *plugin.FakeContextWithEVM, spender diadem.Address, amount *big.Int) error {
	return ec.Contract.Approve(ec.ContractCtx(ctx), &coin.ApproveRequest{
		Spender: spender.MarshalPB(),
		Amount:  &types.BigUInt{Value: *diadem.NewBigUInt(amount)},
	})
}

func (ec *testDiademCoinContract) transfer(ctx *plugin.FakeContextWithEVM, to diadem.Address, amount *big.Int) error {
	return ec.Contract.Transfer(ec.ContractCtx(ctx), &coin.TransferRequest{
		To:     to.MarshalPB(),
		Amount: &types.BigUInt{Value: *diadem.NewBigUInt(amount)},
	})
}

func deployTokenContract(ctx *plugin.FakeContextWithEVM, filename string, gateway, caller diadem.Address) (diadem.Address, error) {
	contractAddr := diadem.Address{}
	hexByteCode, err := ioutil.ReadFile("testdata/" + filename + ".bin")
	if err != nil {
		return contractAddr, err
	}
	abiBytes, err := ioutil.ReadFile("testdata/" + filename + ".abi")
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

// Returns true if seen tx hash
func seenTxHashExist(ctx contract.StaticContext, txHash []byte) bool {
	return ctx.Has(seenTxHashKey(txHash))
}
