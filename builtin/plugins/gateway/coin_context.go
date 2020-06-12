package gateway

import (
	"math/big"

	diadem "github.com/diademnetwork/go-diadem"
	"github.com/diademnetwork/go-diadem/builtin/types/coin"
	contract "github.com/diademnetwork/go-diadem/plugin/contractpb"
	"github.com/diademnetwork/go-diadem/types"
)

// Helper for making static calls into the Go contract that stores diadem native coins on the DAppChain.
type coinStaticContext struct {
	ctx          contract.StaticContext
	contractAddr diadem.Address
}

func newcoinStaticContext(ctx contract.StaticContext) *coinStaticContext {
	contractAddr, err := ctx.Resolve("coin")
	if err != nil {
		panic(err)
	}
	return &coinStaticContext{
		ctx:          ctx,
		contractAddr: contractAddr,
	}
}

func (c *coinStaticContext) balanceOf(owner diadem.Address) (*big.Int, error) {
	req := &coin.BalanceOfRequest{
		Owner: owner.MarshalPB(),
	}
	var resp coin.BalanceOfResponse
	err := contract.StaticCallMethod(c.ctx, c.contractAddr, "BalanceOf", req, &resp)
	if err != nil {
		return nil, err
	}

	if resp.Balance != nil {
		return resp.Balance.Value.Int, nil
	}
	return nil, nil
}

// Helper for making calls into the Go contract that stores native diadem coins on the DAppChain.
type coinContext struct {
	*coinStaticContext
	ctx contract.Context
}

func newCoinContext(ctx contract.Context) *coinContext {
	return &coinContext{
		coinStaticContext: newcoinStaticContext(ctx),
		ctx:               ctx,
	}
}

func (c *coinContext) transferFrom(from, to diadem.Address, amount *big.Int) error {
	req := &coin.TransferFromRequest{
		From:   from.MarshalPB(),
		To:     to.MarshalPB(),
		Amount: &types.BigUInt{Value: *diadem.NewBigUInt(amount)},
	}

	err := contract.CallMethod(c.ctx, c.contractAddr, "TransferFrom", req, nil)
	if err != nil {
		return err
	}

	return nil
}

func (c *coinContext) transfer(to diadem.Address, amount *big.Int) error {
	req := &coin.TransferRequest{
		To:     to.MarshalPB(),
		Amount: &types.BigUInt{Value: *diadem.NewBigUInt(amount)},
	}

	err := contract.CallMethod(c.ctx, c.contractAddr, "Transfer", req, nil)
	if err != nil {
		return err
	}

	return nil
}

func (c *coinContext) burn(ownerAddr diadem.Address, amount *big.Int) error {
	req := &coin.BurnRequest{
		Owner:  ownerAddr.MarshalPB(),
		Amount: &types.BigUInt{Value: *diadem.NewBigUInt(amount)},
	}

	err := contract.CallMethod(c.ctx, c.contractAddr, "Burn", req, nil)
	if err != nil {
		return err
	}

	return nil
}

func (c *coinContext) mintToGateway(amount *big.Int) error {
	req := &coin.MintToGatewayRequest{
		Amount: &types.BigUInt{Value: *diadem.NewBigUInt(amount)},
	}

	err := contract.CallMethod(c.ctx, c.contractAddr, "MintToGateway", req, nil)
	if err != nil {
		return err
	}

	return nil
}
