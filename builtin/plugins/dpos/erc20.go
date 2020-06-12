package dpos

import (
	diadem "github.com/diademnetwork/go-diadem"
	"github.com/diademnetwork/go-diadem/builtin/types/coin"
	contract "github.com/diademnetwork/go-diadem/plugin/contractpb"
	"github.com/diademnetwork/go-diadem/types"
)

type ERC20Static struct {
	StaticContext   contract.StaticContext
	ContractAddress diadem.Address
}

func (c *ERC20Static) TotalSupply() (*diadem.BigUInt, error) {
	req := &coin.TotalSupplyRequest{}
	var resp coin.TotalSupplyResponse

	err := contract.StaticCallMethod(c.StaticContext, c.ContractAddress, "TotalSupply", req, &resp)
	if err != nil {
		return nil, err
	}

	return &resp.TotalSupply.Value, nil
}

func (c *ERC20Static) BalanceOf(addr diadem.Address) (*diadem.BigUInt, error) {
	req := &coin.BalanceOfRequest{
		Owner: addr.MarshalPB(),
	}
	var resp coin.BalanceOfResponse
	err := contract.StaticCallMethod(c.StaticContext, c.ContractAddress, "BalanceOf", req, &resp)
	if err != nil {
		return nil, err
	}

	return &resp.Balance.Value, nil
}

type ERC20 struct {
	Context         contract.Context
	ContractAddress diadem.Address
}

func (c *ERC20) Transfer(to diadem.Address, amount *diadem.BigUInt) error {
	req := &coin.TransferRequest{
		To: to.MarshalPB(),
		Amount: &types.BigUInt{
			Value: *amount,
		},
	}

	err := contract.CallMethod(c.Context, c.ContractAddress, "Transfer", req, nil)
	if err != nil {
		return err
	}

	return nil
}

func (c *ERC20) TransferFrom(from, to diadem.Address, amount *diadem.BigUInt) error {
	req := &coin.TransferFromRequest{
		From: from.MarshalPB(),
		To:   to.MarshalPB(),
		Amount: &types.BigUInt{
			Value: *amount,
		},
	}

	err := contract.CallMethod(c.Context, c.ContractAddress, "TransferFrom", req, nil)
	if err != nil {
		return err
	}

	return nil
}

func (c *ERC20) Approve(spender diadem.Address, amount *diadem.BigUInt) error {
	req := &coin.ApproveRequest{
		Spender: spender.MarshalPB(),
		Amount: &types.BigUInt{
			Value: *amount,
		},
	}

	err := contract.CallMethod(c.Context, c.ContractAddress, "Approve", req, nil)
	if err != nil {
		return err
	}

	return nil
}
