package dposv3

import (
	"errors"
	"math/big"

	diadem "github.com/diademnetwork/go-diadem"
	"github.com/diademnetwork/go-diadem/common"
	contract "github.com/diademnetwork/go-diadem/plugin/contractpb"
)

var TierMap = map[uint64]LocktimeTier{
	0: TIER_ZERO,
	1: TIER_ONE,
	2: TIER_TWO,
	3: TIER_THREE,
}

var TierLocktimeMap = map[LocktimeTier]uint64{
	TIER_ZERO:  1209600,  // two weeks
	TIER_ONE:   7884000,  // three months
	TIER_TWO:   15768000, // six months
	TIER_THREE: 31536000, // one year
}

var TierBonusMap = map[LocktimeTier]diadem.BigUInt{
	TIER_ZERO:  diadem.BigUInt{big.NewInt(10000)}, // two weeks
	TIER_ONE:   diadem.BigUInt{big.NewInt(15000)}, // three months
	TIER_TWO:   diadem.BigUInt{big.NewInt(20000)}, // six months
	TIER_THREE: diadem.BigUInt{big.NewInt(40000)}, // one year
}

// frac is expressed in basis points
func CalculateFraction(frac diadem.BigUInt, total diadem.BigUInt) diadem.BigUInt {
	return CalculatePreciseFraction(basisPointsToBillionths(frac), total)
}

// frac is expressed in billionths
func CalculatePreciseFraction(frac diadem.BigUInt, total diadem.BigUInt) diadem.BigUInt {
	updatedAmount := *common.BigZero()
	updatedAmount.Mul(&total, &frac)
	updatedAmount.Div(&updatedAmount, &billionth)
	return updatedAmount
}

func calculateShare(delegation diadem.BigUInt, total diadem.BigUInt, rewards diadem.BigUInt) diadem.BigUInt {
	frac := common.BigZero()
	if !common.IsZero(total) {
		frac.Mul(&delegation, &billionth)
		frac.Div(frac, &total)
	}
	return CalculatePreciseFraction(*frac, rewards)
}

func scientificNotation(m, n int64) *diadem.BigUInt {
	ret := diadem.NewBigUIntFromInt(10)
	ret.Exp(ret, diadem.NewBigUIntFromInt(n), nil)
	ret.Mul(ret, diadem.NewBigUIntFromInt(m))
	return ret
}

// Locktime Tiers are enforced to be 0-3 for 5-20% rewards. Any other number is reset to 5%. We add the check just in case somehow the variable gets misset.
func calculateWeightedDelegationAmount(delegation Delegation) diadem.BigUInt {
	bonusPercentage, found := TierBonusMap[delegation.LocktimeTier]
	if !found {
		bonusPercentage = TierBonusMap[TIER_ZERO]
	}
	return CalculateFraction(bonusPercentage, delegation.Amount.Value)
}

// Locktime Tiers are enforced to be 0-3 for 5-20% rewards. Any other number is reset to 5%. We add the check just in case somehow the variable gets misset.
func calculateWeightedWhitelistAmount(statistic ValidatorStatistic) diadem.BigUInt {
	bonusPercentage, found := TierBonusMap[statistic.LocktimeTier]
	if !found {
		bonusPercentage = TierBonusMap[TIER_ZERO]
	}
	return CalculateFraction(bonusPercentage, statistic.WhitelistAmount.Value)
}

func basisPointsToBillionths(bps diadem.BigUInt) diadem.BigUInt {
	updatedAmount := diadem.BigUInt{big.NewInt(billionthsBasisPointRatio)}
	updatedAmount.Mul(&updatedAmount, &bps)
	return updatedAmount
}

// VALIDATION

func validateFee(fee uint64) error {
	if fee > 10000 {
		return errors.New("Fee percentage cannot be greater than 100%.")
	}

	return nil
}

// LOGGING

func logDposError(ctx contract.Context, err error, req string) error {
	ctx.Logger().Error("DPOS error", "error", err, "sender", ctx.Message().Sender, "req", req)
	return err
}

func logStaticDposError(ctx contract.StaticContext, err error, req string) error {
	ctx.Logger().Error("DPOS static error", "error", err, "sender", ctx.Message().Sender, "req", req)
	return err
}
