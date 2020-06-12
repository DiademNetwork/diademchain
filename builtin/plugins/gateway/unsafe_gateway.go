// +build evm

package gateway

import (
	diadem "github.com/diademnetwork/go-diadem"
	tgtypes "github.com/diademnetwork/go-diadem/builtin/types/transfer_gateway"
	contract "github.com/diademnetwork/go-diadem/plugin/contractpb"
)

type (
	ResetMainnetBlockRequest = tgtypes.TransferGatewayResetMainnetBlockRequest
)

type UnsafeGateway struct {
	Gateway
}

func (gw *UnsafeGateway) ResetMainnetBlock(ctx contract.Context, req *ResetMainnetBlockRequest) error {
	state, err := loadState(ctx)
	if err != nil {
		return err
	}

	state.LastMainnetBlockNum = req.GetLastMainnetBlockNum()

	return saveState(ctx, state)
}

func (gw *UnsafeGateway) UnsafeAddOracle(ctx contract.Context, req *AddOracleRequest) error {
	oracleAddr := diadem.UnmarshalAddressPB(req.Oracle)
	if ctx.Has(oracleStateKey(oracleAddr)) {
		return ErrOracleAlreadyRegistered
	}

	return addOracle(ctx, oracleAddr)
}

func (gw *UnsafeGateway) UnsafeRemoveOracle(ctx contract.Context, req *RemoveOracleRequest) error {
	oracleAddr := diadem.UnmarshalAddressPB(req.Oracle)
	if !ctx.Has(oracleStateKey(oracleAddr)) {
		return ErrOracleNotRegistered
	}

	return removeOracle(ctx, oracleAddr)
}

func (gw *UnsafeGateway) ResetOwnerKey(ctx contract.Context, req *AddOracleRequest) error {
	state, err := loadState(ctx)
	if err != nil {
		return err
	}

	// Revoke permissions from old owner
	oldOwnerAddr := diadem.UnmarshalAddressPB(state.Owner)
	ctx.RevokePermissionFrom(oldOwnerAddr, changeOraclesPerm, ownerRole)

	// Update owner and grant permissions
	state.Owner = req.Oracle
	ownerAddr := diadem.UnmarshalAddressPB(req.Oracle)
	ctx.GrantPermissionTo(ownerAddr, changeOraclesPerm, ownerRole)

	return saveState(ctx, state)
}
