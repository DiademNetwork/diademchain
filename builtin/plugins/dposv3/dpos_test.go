package dposv3

import (
	"encoding/hex"
	"math/big"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	diadem "github.com/diademnetwork/go-diadem"
	common "github.com/diademnetwork/go-diadem/common"
	"github.com/diademnetwork/go-diadem/plugin"
	"github.com/diademnetwork/go-diadem/plugin/contractpb"
	types "github.com/diademnetwork/go-diadem/types"
	"github.com/diademnetwork/diademchain"
	"github.com/diademnetwork/diademchain/builtin/plugins/coin"
)

var (
	validatorPubKeyHex1 = "3866f776276246e4f9998aa90632931d89b0d3a5930e804e02299533f55b39e1"
	validatorPubKeyHex2 = "7796b813617b283f81ea1747fbddbe73fe4b5fce0eac0728e47de51d8e506701"
	validatorPubKeyHex3 = "e4008e26428a9bca87465e8de3a8d0e9c37a56ca619d3d6202b0567528786618"
	validatorPubKeyHex4 = "21908210428a9bca87465e8de3a8d0e9c37a56ca619d3d6202b0567528786618"

	delegatorAddress1       = diadem.MustParseAddress("chain:0xb16a379ec18d4093666f8f38b11a3071c920207d")
	delegatorAddress2       = diadem.MustParseAddress("chain:0xfa4c7920accfd66b86f5fd0e69682a79f762d49e")
	delegatorAddress3       = diadem.MustParseAddress("chain:0x5cecd1f7261e1f4c684e297be3edf03b825e01c4")
	delegatorAddress4       = diadem.MustParseAddress("chain:0x000000000000000000000000e3edf03b825e01e0")
	delegatorAddress5       = diadem.MustParseAddress("chain:0x020000000000000000000000e3edf03b825e0288")
	delegatorAddress6       = diadem.MustParseAddress("chain:0x000000000000000000040400e3edf03b825e0398")
	chainID                 = "default"
	startTime         int64 = 100000

	pubKey1, _ = hex.DecodeString(validatorPubKeyHex1)
	addr1      = diadem.Address{
		ChainID: chainID,
		Local:   diadem.LocalAddressFromPublicKey(pubKey1),
	}
	pubKey2, _ = hex.DecodeString(validatorPubKeyHex2)
	addr2      = diadem.Address{
		ChainID: chainID,
		Local:   diadem.LocalAddressFromPublicKey(pubKey2),
	}
	pubKey3, _ = hex.DecodeString(validatorPubKeyHex3)
	addr3      = diadem.Address{
		ChainID: chainID,
		Local:   diadem.LocalAddressFromPublicKey(pubKey3),
	}
	pubKey4, _ = hex.DecodeString(validatorPubKeyHex4)
	addr4      = diadem.Address{
		ChainID: chainID,
		Local:   diadem.LocalAddressFromPublicKey(pubKey4),
	}
)

func TestRegisterWhitelistedCandidate(t *testing.T) {
	oraclePubKey, _ := hex.DecodeString(validatorPubKeyHex2)
	oracleAddr := diadem.Address{
		Local: diadem.LocalAddressFromPublicKey(oraclePubKey),
	}

	pubKey, _ := hex.DecodeString(validatorPubKeyHex1)
	addr := diadem.Address{
		Local: diadem.LocalAddressFromPublicKey(pubKey),
	}
	pctx := plugin.CreateFakeContext(addr, addr)

	coinContract := &coin.Coin{}
	coinAddr := pctx.CreateContract(coin.Contract)
	coinCtx := pctx.WithAddress(coinAddr)
	coinContract.Init(contractpb.WrapPluginContext(coinCtx), &coin.InitRequest{
		Accounts: []*coin.InitialAccount{
			makeAccount(addr2, 2000000000000000000),
		},
	})

	dpos, err := deployDPOSContract(pctx, &Params{
		ValidatorCount:      21,
		CoinContractAddress: coinAddr.MarshalPB(),
		OracleAddress:       oracleAddr.MarshalPB(),
	})
	require.Nil(t, err)

	whitelistAmount := big.NewInt(1000000000000)
	err = dpos.WhitelistCandidate(pctx.WithSender(oracleAddr), addr, whitelistAmount, 0)
	require.Nil(t, err)

	err = dpos.RegisterCandidate(pctx.WithSender(addr), pubKey, nil, nil, nil, nil, nil, nil)
	require.Nil(t, err)

	err = dpos.UnregisterCandidate(pctx.WithSender(addr))
	require.Nil(t, err)

	require.NoError(t, elect(pctx, dpos.Address))

	registrationFee := &types.BigUInt{Value: *scientificNotation(defaultRegistrationRequirement, tokenDecimals)}
	err = coinContract.Approve(contractpb.WrapPluginContext(coinCtx.WithSender(addr2)), &coin.ApproveRequest{
		Spender: dpos.Address.MarshalPB(),
		Amount:  registrationFee,
	})
	require.Nil(t, err)

	err = dpos.RegisterCandidate(pctx.WithSender(addr2), pubKey2, nil, nil, nil, nil, nil, nil)
	require.Nil(t, err)

	err = dpos.RegisterCandidate(pctx.WithSender(addr), pubKey, nil, nil, nil, nil, nil, nil)
	require.Nil(t, err)

	err = dpos.RemoveWhitelistedCandidate(pctx.WithSender(oracleAddr), &addr)
	require.Nil(t, err)

	candidates, err := dpos.ListCandidates(pctx)
	require.Nil(t, err)
	assert.Equal(t, 2, len(candidates))

	err = dpos.UnregisterCandidate(pctx.WithSender(addr))
	require.Nil(t, err)

	candidates, err = dpos.ListCandidates(pctx)
	require.Nil(t, err)
	assert.Equal(t, 2, len(candidates))

	require.NoError(t, elect(pctx, dpos.Address))

	candidates, err = dpos.ListCandidates(pctx)
	require.Nil(t, err)
	assert.Equal(t, 1, len(candidates))

	err = dpos.RegisterCandidate(pctx.WithSender(addr), pubKey, nil, nil, nil, nil, nil, nil)
	require.NotNil(t, err)
}

func TestChangeFee(t *testing.T) {
	oldFee := uint64(100)
	newFee := uint64(1000)
	oraclePubKey, _ := hex.DecodeString(validatorPubKeyHex2)
	oracleAddr := diadem.Address{
		Local: diadem.LocalAddressFromPublicKey(oraclePubKey),
	}

	pubKey, _ := hex.DecodeString(validatorPubKeyHex1)
	addr := diadem.Address{
		Local: diadem.LocalAddressFromPublicKey(pubKey),
	}
	pctx := plugin.CreateFakeContext(addr, addr)

	// Deploy the coin contract (DPOS Init() will attempt to resolve it)
	coinContract := &coin.Coin{}
	_ = pctx.CreateContract(contractpb.MakePluginContract(coinContract))

	dpos, err := deployDPOSContract(pctx, &Params{
		ValidatorCount: 21,
		OracleAddress:  oracleAddr.MarshalPB(),
	})
	require.Nil(t, err)

	amount := big.NewInt(1000000000000)
	err = dpos.WhitelistCandidate(pctx.WithSender(oracleAddr), addr, amount, 0)
	require.Nil(t, err)

	err = dpos.RegisterCandidate(pctx.WithSender(addr), pubKey, nil, &oldFee, nil, nil, nil, nil)
	require.Nil(t, err)

	candidates, err := dpos.ListCandidates(pctx)
	require.Nil(t, err)
	assert.Equal(t, oldFee, candidates[0].Candidate.Fee)
	assert.Equal(t, oldFee, candidates[0].Candidate.NewFee)

	require.NoError(t, elect(pctx, dpos.Address))

	require.NoError(t, elect(pctx, dpos.Address))

	candidates, err = dpos.ListCandidates(pctx)
	require.Nil(t, err)
	assert.Equal(t, oldFee, candidates[0].Candidate.Fee)
	assert.Equal(t, oldFee, candidates[0].Candidate.NewFee)

	err = dpos.ChangeFee(pctx.WithSender(addr), newFee)
	require.Nil(t, err)

	require.NoError(t, elect(pctx, dpos.Address))

	candidates, err = dpos.ListCandidates(pctx)
	require.Nil(t, err)
	// Fee should not reset after only a single election
	assert.Equal(t, oldFee, candidates[0].Candidate.Fee)
	assert.Equal(t, newFee, candidates[0].Candidate.NewFee)

	require.NoError(t, elect(pctx, dpos.Address))

	candidates, err = dpos.ListCandidates(pctx)
	require.Nil(t, err)
	// Fee should reset after two elections
	assert.Equal(t, newFee, candidates[0].Candidate.Fee)
	assert.Equal(t, newFee, candidates[0].Candidate.NewFee)
}

func TestDelegate(t *testing.T) {
	pctx := plugin.CreateFakeContext(addr1, addr1)

	oraclePubKey, _ := hex.DecodeString(validatorPubKeyHex2)
	oracleAddr := diadem.Address{
		Local: diadem.LocalAddressFromPublicKey(oraclePubKey),
	}

	// Deploy the coin contract (DPOS Init() will attempt to resolve it)
	coinContract := &coin.Coin{}
	coinAddr := pctx.CreateContract(coin.Contract)
	coinCtx := pctx.WithAddress(coinAddr)
	coinContract.Init(contractpb.WrapPluginContext(coinCtx), &coin.InitRequest{
		Accounts: []*coin.InitialAccount{
			makeAccount(delegatorAddress1, 1000000000000000000),
			makeAccount(delegatorAddress2, 2000000000000000000),
			makeAccount(delegatorAddress3, 1000000000000000000),
			makeAccount(addr1, 1000000000000000000),
		},
	})

	dpos, err := deployDPOSContract(pctx, &Params{
		ValidatorCount: 21,
		OracleAddress:  oracleAddr.MarshalPB(),
	})
	require.Nil(t, err)

	whitelistAmount := big.NewInt(1000000000000)
	// should fail from non-oracle
	err = dpos.WhitelistCandidate(pctx.WithSender(addr1), addr1, whitelistAmount, 0)
	require.Error(t, err)

	err = dpos.WhitelistCandidate(pctx.WithSender(oracleAddr), addr1, whitelistAmount, 0)
	require.Nil(t, err)

	err = dpos.RegisterCandidate(pctx.WithSender(addr1), pubKey1, nil, nil, nil, nil, nil, nil)
	require.Nil(t, err)

	delegationAmount := big.NewInt(100)
	err = coinContract.Approve(contractpb.WrapPluginContext(coinCtx.WithSender(addr1)), &coin.ApproveRequest{
		Spender: dpos.Address.MarshalPB(),
		Amount:  &types.BigUInt{Value: *diadem.NewBigUInt(delegationAmount)},
	})
	require.Nil(t, err)

	response, err := coinContract.Allowance(contractpb.WrapPluginContext(coinCtx.WithSender(oracleAddr)), &coin.AllowanceRequest{
		Owner:   addr1.MarshalPB(),
		Spender: dpos.Address.MarshalPB(),
	})
	require.Nil(t, err)
	require.True(t, delegationAmount.Cmp(response.Amount.Value.Int) == 0)

	candidates, err := dpos.ListCandidates(pctx)
	require.Nil(t, err)
	assert.Equal(t, len(candidates), 1)

	err = dpos.Delegate(pctx.WithSender(addr1), &addr1, delegationAmount, nil, nil)
	require.Nil(t, err)

	err = coinContract.Approve(contractpb.WrapPluginContext(coinCtx.WithSender(addr1)), &coin.ApproveRequest{
		Spender: dpos.Address.MarshalPB(),
		Amount:  &types.BigUInt{Value: *diadem.NewBigUInt(delegationAmount)},
	})
	require.Nil(t, err)

	// total rewards distribution should equal 0 before elections run
	totalRewardDistribution, err := dpos.CheckRewards(pctx.WithSender(addr1))
	require.Nil(t, err)
	assert.True(t, totalRewardDistribution.Cmp(common.BigZero()) == 0)

	require.NoError(t, elect(pctx, dpos.Address))

	// total rewards distribution should equal still be zero after first election
	totalRewardDistribution, err = dpos.CheckRewards(pctx.WithSender(addr1))
	require.Nil(t, err)
	assert.True(t, totalRewardDistribution.Cmp(common.BigZero()) == 0)

	err = dpos.Delegate(pctx.WithSender(addr1), &addr1, delegationAmount, nil, nil)
	require.Nil(t, err)

	_, delegatedAmount, _, err := dpos.CheckDelegation(pctx, &addr1, &addr2)
	require.Nil(t, err)
	assert.True(t, delegatedAmount.Cmp(big.NewInt(0)) == 0)

	err = coinContract.Approve(contractpb.WrapPluginContext(coinCtx.WithSender(delegatorAddress1)), &coin.ApproveRequest{
		Spender: dpos.Address.MarshalPB(),
		Amount:  &types.BigUInt{Value: *diadem.NewBigUInt(delegationAmount)},
	})
	require.Nil(t, err)

	err = dpos.Delegate(pctx.WithSender(delegatorAddress1), &addr1, delegationAmount, nil, nil)
	require.Nil(t, err)

	// checking a non-existent delegation should result in an empty (amount = 0)
	// delegaiton being returned
	_, delegatedAmount, _, err = dpos.CheckDelegation(pctx, &addr1, &addr2)
	require.Nil(t, err)
	assert.True(t, delegatedAmount.Cmp(big.NewInt(0)) == 0)

	err = coinContract.Approve(contractpb.WrapPluginContext(coinCtx.WithSender(addr1)), &coin.ApproveRequest{
		Spender: dpos.Address.MarshalPB(),
		Amount:  &types.BigUInt{Value: *diadem.NewBigUInt(delegationAmount)},
	})
	require.Nil(t, err)

	require.NoError(t, elect(pctx, dpos.Address))

	// total rewards distribution should be greater than zero
	totalRewardDistribution, err = dpos.CheckRewards(pctx.WithSender(addr1))
	require.Nil(t, err)
	assert.True(t, common.IsPositive(*totalRewardDistribution))

	// advancing contract time beyond the delegator1-addr1 lock period
	now := uint64(pctx.Now().Unix())
	pctx.SetTime(pctx.Now().Add(time.Duration(now+TierLocktimeMap[0]) * time.Second))

	err = dpos.Unbond(pctx.WithSender(addr1), &addr1, delegationAmount, 1)
	require.Nil(t, err)

	require.NoError(t, elect(pctx, dpos.Address))

	err = dpos.Unbond(pctx.WithSender(addr1), &addr1, delegationAmount, 2)
	require.Nil(t, err)

	require.NoError(t, elect(pctx, dpos.Address))

	err = dpos.Unbond(pctx.WithSender(addr1), &addr1, big.NewInt(1), 3)
	require.Error(t, err)

	// testing delegations to limbo validator
	err = dpos.Redelegate(pctx.WithSender(delegatorAddress1), &addr1, &limboValidatorAddress, delegationAmount, 1, nil, nil)
	require.Nil(t, err)

	require.NoError(t, elect(pctx, dpos.Address))

	_, delegatedAmount, _, err = dpos.CheckDelegation(pctx, &addr1, &delegatorAddress1)
	require.Nil(t, err)
	assert.True(t, delegatedAmount.Cmp(big.NewInt(0)) == 0)

	_, delegatedAmount, _, err = dpos.CheckDelegation(pctx, &limboValidatorAddress, &delegatorAddress1)
	require.Nil(t, err)
	assert.True(t, delegatedAmount.Cmp(delegationAmount) == 0)
}

func TestRedelegate(t *testing.T) {
	pctx := plugin.CreateFakeContext(addr1, addr1)

	// Deploy the coin contract (DPOS Init() will attempt to resolve it)
	coinContract := &coin.Coin{}
	coinAddr := pctx.CreateContract(coin.Contract)
	coinCtx := pctx.WithAddress(coinAddr)
	coinContract.Init(contractpb.WrapPluginContext(coinCtx), &coin.InitRequest{
		Accounts: []*coin.InitialAccount{
			makeAccount(delegatorAddress1, 1000000000000000000),
			makeAccount(delegatorAddress2, 2000000000000000000),
			makeAccount(delegatorAddress3, 1000000000000000000),
			makeAccount(addr1, 1000000000000000000),
			makeAccount(addr2, 1000000000000000000),
			makeAccount(addr3, 1000000000000000000),
		},
	})

	registrationFee := diadem.BigZeroPB()
	dpos, err := deployDPOSContract(pctx, &Params{
		ValidatorCount:          21,
		RegistrationRequirement: registrationFee,
	})
	require.Nil(t, err)

	// Registering 3 candidates
	err = dpos.RegisterCandidate(pctx.WithSender(addr1), pubKey1, nil, nil, nil, nil, nil, nil)
	require.Nil(t, err)

	err = dpos.RegisterCandidate(pctx.WithSender(addr2), pubKey2, nil, nil, nil, nil, nil, nil)
	require.Nil(t, err)

	err = dpos.RegisterCandidate(pctx.WithSender(addr3), pubKey3, nil, nil, nil, nil, nil, nil)
	require.Nil(t, err)

	candidates, err := dpos.ListCandidates(pctx)
	require.Nil(t, err)
	assert.Equal(t, len(candidates), 3)

	require.NoError(t, elect(pctx, dpos.Address))

	// Verifying that with registration fee = 0, none of the 3 registered candidates are elected validators
	validators, err := dpos.ListValidators(pctx)
	require.Nil(t, err)
	assert.Equal(t, len(validators), 0)

	delegationAmount := big.NewInt(10000000)
	smallDelegationAmount := big.NewInt(1000000)

	err = coinContract.Approve(contractpb.WrapPluginContext(coinCtx.WithSender(delegatorAddress1)), &coin.ApproveRequest{
		Spender: dpos.Address.MarshalPB(),
		Amount:  &types.BigUInt{Value: *diadem.NewBigUInt(delegationAmount)},
	})
	require.Nil(t, err)

	err = dpos.Delegate(pctx.WithSender(delegatorAddress1), &addr1, delegationAmount, nil, nil)
	require.Nil(t, err)

	require.NoError(t, elect(pctx, dpos.Address))

	// Verifying that addr1 was elected sole validator
	validators, err = dpos.ListValidators(pctx)
	require.Nil(t, err)
	assert.Equal(t, len(validators), 1)
	assert.True(t, validators[0].Address.Local.Compare(addr1.Local) == 0)

	// checking that redelegation fails with 0 amount
	err = dpos.Redelegate(pctx.WithSender(delegatorAddress1), &addr1, &addr2, big.NewInt(0), 1, nil, nil)
	require.NotNil(t, err)

	// redelegating sole delegation to validator addr2
	err = dpos.Redelegate(pctx.WithSender(delegatorAddress1), &addr1, &addr2, delegationAmount, 1, nil, nil)
	require.Nil(t, err)

	// Redelegation takes effect within a single election period
	require.NoError(t, elect(pctx, dpos.Address))

	// Verifying that addr2 was elected sole validator
	validators, err = dpos.ListValidators(pctx)
	require.Nil(t, err)
	assert.Equal(t, len(validators), 1)
	assert.True(t, validators[0].Address.Local.Compare(addr2.Local) == 0)

	// redelegating sole delegation to validator addr3
	err = dpos.Redelegate(pctx.WithSender(delegatorAddress1), &addr2, &addr3, delegationAmount, 1, nil, nil)
	require.Nil(t, err)

	// Redelegation takes effect within a single election period
	require.NoError(t, elect(pctx, dpos.Address))

	// Verifying that addr3 was elected sole validator
	validators, err = dpos.ListValidators(pctx)
	require.Nil(t, err)
	assert.Equal(t, len(validators), 1)
	assert.True(t, validators[0].Address.Local.Compare(addr3.Local) == 0)

	err = coinContract.Approve(contractpb.WrapPluginContext(coinCtx.WithSender(delegatorAddress2)), &coin.ApproveRequest{
		Spender: dpos.Address.MarshalPB(),
		Amount:  &types.BigUInt{Value: *diadem.NewBigUInt(delegationAmount)},
	})
	require.Nil(t, err)

	// adding 2nd delegation from 2nd delegator in order to elect a second validator
	err = dpos.Delegate(pctx.WithSender(delegatorAddress2), &addr1, delegationAmount, nil, nil)
	require.Nil(t, err)

	require.NoError(t, elect(pctx, dpos.Address))

	// checking that the 2nd validator (addr1) was elected in addition to add3
	validators, err = dpos.ListValidators(pctx)
	require.Nil(t, err)
	assert.Equal(t, len(validators), 2)

	// delegator 1 removes delegation to limbo
	err = dpos.Redelegate(pctx.WithSender(delegatorAddress1), &addr3, &limboValidatorAddress, delegationAmount, 1, nil, nil)
	require.Nil(t, err)

	require.NoError(t, elect(pctx, dpos.Address))

	// Verifying that addr1 was elected sole validator AFTER delegator1 redelegated to limbo validator
	validators, err = dpos.ListValidators(pctx)
	require.Nil(t, err)
	assert.Equal(t, len(validators), 1)
	assert.True(t, validators[0].Address.Local.Compare(addr1.Local) == 0)

	// Checking that redelegaiton of a negative amount is rejected
	err = dpos.Redelegate(pctx.WithSender(delegatorAddress2), &addr1, &addr2, big.NewInt(-1000), 1, nil, nil)
	require.NotNil(t, err)

	// Checking that redelegaiton of an amount greater than the total delegation is rejected
	err = dpos.Redelegate(pctx.WithSender(delegatorAddress2), &addr1, &addr2, big.NewInt(100000000), 1, nil, nil)
	require.NotNil(t, err)

	// splitting delegator2's delegation to 2nd validator
	err = dpos.Redelegate(pctx.WithSender(delegatorAddress2), &addr1, &addr2, smallDelegationAmount, 1, nil, nil)
	require.Nil(t, err)

	// splitting delegator2's delegation to 3rd validator
	// this also tests that redelegate is able to set a new tier
	tier := uint64(3)
	err = dpos.Redelegate(pctx.WithSender(delegatorAddress2), &addr1, &addr3, smallDelegationAmount, 1, &tier, nil)
	require.Nil(t, err)

	require.NoError(t, elect(pctx, dpos.Address))

	delegations, _, _, err := dpos.CheckDelegation(pctx, &addr3, &delegatorAddress2)
	require.Nil(t, err)
	// assert.True(t, delegationResponse.Amount.Value.Cmp(smallDelegationAmount) == 0)
	assert.Equal(t, delegations[len(delegations)-1].LocktimeTier, TIER_THREE)

	// checking that all 3 candidates have been elected validators
	validators, err = dpos.ListValidators(pctx)
	require.Nil(t, err)
	assert.Equal(t, len(validators), 3)
}

func TestReward(t *testing.T) {
	// set elect time in params to one second for easy calculations
	delegationAmount := diadem.BigUInt{big.NewInt(10000000000000)}
	cycleLengthSeconds := int64(100)
	params := Params{
		ElectionCycleLength: cycleLengthSeconds,
		MaxYearlyReward:     &types.BigUInt{Value: *scientificNotation(defaultMaxYearlyReward, tokenDecimals)},
	}
	statistic := ValidatorStatistic{
		DelegationTotal: &types.BigUInt{Value: delegationAmount},
	}

	rewardTotal := common.BigZero()
	for i := int64(0); i < yearSeconds; i = i + cycleLengthSeconds {
		cycleReward := calculateRewards(statistic.DelegationTotal.Value, &params, *common.BigZero())
		rewardTotal.Add(rewardTotal, &cycleReward)
	}

	// checking that distribution is roughtly equal to 5% of delegation after one year
	assert.Equal(t, rewardTotal.Cmp(&diadem.BigUInt{big.NewInt(490000000000)}), 1)
	assert.Equal(t, rewardTotal.Cmp(&diadem.BigUInt{big.NewInt(510000000000)}), -1)
}

func TestElectWhitelists(t *testing.T) {
	// Init the coin balances
	pctx := plugin.CreateFakeContext(delegatorAddress1, diadem.Address{}).WithBlock(diadem.BlockHeader{
		ChainID: chainID,
		Time:    startTime,
	})
	coinAddr := pctx.CreateContract(coin.Contract)

	coinContract := &coin.Coin{}
	coinCtx := pctx.WithAddress(coinAddr)
	coinContract.Init(contractpb.WrapPluginContext(coinCtx), &coin.InitRequest{
		Accounts: []*coin.InitialAccount{
			makeAccount(delegatorAddress1, 1e18),
			makeAccount(delegatorAddress2, 20),
			makeAccount(delegatorAddress3, 10),
		},
	})
	// Enable the feature flag and check that the whitelist rules get applied corectly
	cycleLengthSeconds := int64(100)
	maxYearlyReward := scientificNotation(defaultMaxYearlyReward, tokenDecimals)
	// Init the dpos contract
	dpos, err := deployDPOSContract(pctx, &Params{
		ValidatorCount:      5,
		ElectionCycleLength: cycleLengthSeconds,
		CoinContractAddress: coinAddr.MarshalPB(),
		MaxYearlyReward:     &types.BigUInt{Value: *maxYearlyReward},
		OracleAddress:       addr1.MarshalPB(),
	})
	require.Nil(t, err)
	dposCtx := pctx.WithAddress(dpos.Address)
	dposCtx.SetFeature(diademchain.DPOSVersion2_1, true)
	require.True(t, dposCtx.FeatureEnabled(diademchain.DPOSVersion2_1, false))

	// transfer coins to reward fund
	amount := big.NewInt(10000000)
	amount.Mul(amount, big.NewInt(1e18))
	err = coinContract.Transfer(contractpb.WrapPluginContext(coinCtx.WithSender(delegatorAddress1)), &coin.TransferRequest{
		To:     dpos.Address.MarshalPB(),
		Amount: &types.BigUInt{Value: diadem.BigUInt{amount}},
	})
	require.Nil(t, err)

	whitelistAmount := big.NewInt(1000000000000)

	// Whitelist with locktime tier 0, which should use 5% of rewards
	err = dpos.WhitelistCandidate(pctx.WithSender(addr1), addr1, whitelistAmount, 0)
	require.Nil(t, err)

	// Whitelist with locktime tier 1, which should use 7.5% of rewards
	err = dpos.WhitelistCandidate(pctx.WithSender(addr1), addr2, whitelistAmount, 1)
	require.Nil(t, err)

	// Whitelist with locktime tier 2, which should use 10% of rewards
	err = dpos.WhitelistCandidate(pctx.WithSender(addr1), addr3, whitelistAmount, 2)
	require.Nil(t, err)

	// Whitelist with locktime tier 3, which should use 20% of rewards
	err = dpos.WhitelistCandidate(pctx.WithSender(addr1), addr4, whitelistAmount, 3)

	// Register the 4 validators
	err = dpos.RegisterCandidate(pctx.WithSender(addr1), pubKey1, nil, nil, nil, nil, nil, nil)
	require.Nil(t, err)

	err = dpos.RegisterCandidate(pctx.WithSender(addr2), pubKey2, nil, nil, nil, nil, nil, nil)
	require.Nil(t, err)

	err = dpos.RegisterCandidate(pctx.WithSender(addr3), pubKey3, nil, nil, nil, nil, nil, nil)
	require.Nil(t, err)

	err = dpos.RegisterCandidate(pctx.WithSender(addr4), pubKey4, nil, nil, nil, nil, nil, nil)
	require.Nil(t, err)

	// Check that they were registered properly
	candidates, err := dpos.ListCandidates(pctx)
	require.Nil(t, err)
	assert.Equal(t, len(candidates), 4)

	validators, err := dpos.ListValidators(pctx)
	require.Nil(t, err)
	assert.Equal(t, len(validators), 0)

	// Elect them
	require.NoError(t, elect(pctx, dpos.Address))

	validators, err = dpos.ListValidators(pctx)
	require.Nil(t, err)
	assert.Equal(t, len(validators), 4)

	// Do a bunch of elections that correspond to 1/100th of a year
	for i := int64(0); i < yearSeconds/100; i = i + cycleLengthSeconds {
		require.NoError(t, elect(pctx, dpos.Address))
		pctx.SetTime(pctx.Now().Add(time.Duration(cycleLengthSeconds) * time.Second))
	}

	rewards1, err := dpos.CheckRewardDelegation(pctx.WithSender(addr1), &addr1)
	require.Nil(t, err)
	// checking that rewards are roughtly equal to 0.5% of delegation after one year
	assert.Equal(t, rewards1.Amount.Value.Cmp(&diadem.BigUInt{big.NewInt(490000000)}), 1)
	assert.Equal(t, rewards1.Amount.Value.Cmp(&diadem.BigUInt{big.NewInt(510000000)}), -1)

	rewards2, err := dpos.CheckRewardDelegation(pctx.WithSender(addr2), &addr2)
	require.Nil(t, err)
	// checking that rewards are roughtly equal to 0.75% of delegation after one year
	assert.Equal(t, rewards2.Amount.Value.Cmp(&diadem.BigUInt{big.NewInt(740000000)}), 1)
	assert.Equal(t, rewards2.Amount.Value.Cmp(&diadem.BigUInt{big.NewInt(760000000)}), -1)

	rewards3, err := dpos.CheckRewardDelegation(pctx.WithSender(addr3), &addr3)
	require.Nil(t, err)
	// checking that rewards are roughtly equal to 1% of delegation after one year
	assert.Equal(t, rewards3.Amount.Value.Cmp(&diadem.BigUInt{big.NewInt(990000000)}), 1)
	assert.Equal(t, rewards3.Amount.Value.Cmp(&diadem.BigUInt{big.NewInt(1000000000)}), -1)

	rewards4, err := dpos.CheckRewardDelegation(pctx.WithSender(addr4), &addr4)
	require.Nil(t, err)
	// checking that rewards are roughtly equal to 2% of delegation after one year
	assert.Equal(t, rewards4.Amount.Value.Cmp(&diadem.BigUInt{big.NewInt(1990000000)}), 1)
	assert.Equal(t, rewards4.Amount.Value.Cmp(&diadem.BigUInt{big.NewInt(2000000000)}), -1)

	// Let's withdraw rewards and see how the balances change.

	err = dpos.Unbond(pctx.WithSender(addr1), &addr1, big.NewInt(0), REWARD_DELEGATION_INDEX)
	require.Nil(t, err)
	err = dpos.Unbond(pctx.WithSender(addr2), &addr2, big.NewInt(0), REWARD_DELEGATION_INDEX)
	require.Nil(t, err)
	err = dpos.Unbond(pctx.WithSender(addr3), &addr3, big.NewInt(0), REWARD_DELEGATION_INDEX)
	require.Nil(t, err)
	err = dpos.Unbond(pctx.WithSender(addr4), &addr4, big.NewInt(0), REWARD_DELEGATION_INDEX)
	require.Nil(t, err)

	require.NoError(t, elect(pctx, dpos.Address))

	balanceAfterClaim, err := coinContract.BalanceOf(contractpb.WrapPluginContext(coinCtx), &coin.BalanceOfRequest{
		Owner: addr1.MarshalPB(),
	})
	require.Nil(t, err)
	assert.Equal(t, balanceAfterClaim.Balance.Value.Cmp(&diadem.BigUInt{big.NewInt(490000000)}), 1)
	assert.Equal(t, balanceAfterClaim.Balance.Value.Cmp(&diadem.BigUInt{big.NewInt(510000000)}), -1)

	balanceAfterClaim, err = coinContract.BalanceOf(contractpb.WrapPluginContext(coinCtx), &coin.BalanceOfRequest{
		Owner: addr2.MarshalPB(),
	})
	require.Nil(t, err)
	assert.Equal(t, balanceAfterClaim.Balance.Value.Cmp(&diadem.BigUInt{big.NewInt(740000000)}), 1)
	assert.Equal(t, balanceAfterClaim.Balance.Value.Cmp(&diadem.BigUInt{big.NewInt(760000000)}), -1)

	balanceAfterClaim, err = coinContract.BalanceOf(contractpb.WrapPluginContext(coinCtx), &coin.BalanceOfRequest{
		Owner: addr3.MarshalPB(),
	})
	require.Nil(t, err)
	assert.Equal(t, balanceAfterClaim.Balance.Value.Cmp(&diadem.BigUInt{big.NewInt(990000000)}), 1)
	assert.Equal(t, balanceAfterClaim.Balance.Value.Cmp(&diadem.BigUInt{big.NewInt(1000000000)}), -1)

	balanceAfterClaim, err = coinContract.BalanceOf(contractpb.WrapPluginContext(coinCtx), &coin.BalanceOfRequest{
		Owner: addr4.MarshalPB(),
	})
	require.Nil(t, err)
	assert.Equal(t, balanceAfterClaim.Balance.Value.Cmp(&diadem.BigUInt{big.NewInt(1990000000)}), 1)
	assert.Equal(t, balanceAfterClaim.Balance.Value.Cmp(&diadem.BigUInt{big.NewInt(2000000000)}), -1)

}

func TestElect(t *testing.T) {
	pctx := plugin.CreateFakeContext(delegatorAddress1, diadem.Address{}).WithBlock(diadem.BlockHeader{
		ChainID: chainID,
		Time:    startTime,
	})
	coinAddr := pctx.CreateContract(coin.Contract)

	coinContract := &coin.Coin{}
	coinCtx := pctx.WithAddress(coinAddr)
	// Initialize the coin balances
	coinContract.Init(contractpb.WrapPluginContext(coinCtx), &coin.InitRequest{
		Accounts: []*coin.InitialAccount{
			makeAccount(delegatorAddress1, 130),
			makeAccount(delegatorAddress2, 20),
			makeAccount(delegatorAddress3, 10),
		},
	})

	// create dpos contract
	dpos, err := deployDPOSContract(pctx, &Params{
		ValidatorCount:      2,
		CoinContractAddress: coinAddr.MarshalPB(),
		OracleAddress:       addr1.MarshalPB(),
	})
	require.Nil(t, err)

	// transfer coins to reward fund
	amount := big.NewInt(10)
	amount.Exp(amount, big.NewInt(19), nil)
	coinContract.Transfer(contractpb.WrapPluginContext(coinCtx), &coin.TransferRequest{
		To: dpos.Address.MarshalPB(),
		Amount: &types.BigUInt{
			Value: common.BigUInt{amount},
		},
	})

	whitelistAmount := big.NewInt(1000000000000)

	err = dpos.WhitelistCandidate(pctx.WithSender(addr1), addr1, whitelistAmount, 0)
	require.Nil(t, err)

	err = dpos.WhitelistCandidate(pctx.WithSender(addr1), addr2, whitelistAmount, 0)
	require.Nil(t, err)

	err = dpos.WhitelistCandidate(pctx.WithSender(addr1), addr3, whitelistAmount, 0)
	require.Nil(t, err)

	err = dpos.RegisterCandidate(pctx.WithSender(addr1), pubKey1, nil, nil, nil, nil, nil, nil)
	require.Nil(t, err)

	err = dpos.RegisterCandidate(pctx.WithSender(addr2), pubKey2, nil, nil, nil, nil, nil, nil)
	require.Nil(t, err)

	err = dpos.RegisterCandidate(pctx.WithSender(addr3), pubKey3, nil, nil, nil, nil, nil, nil)
	require.Nil(t, err)

	candidates, err := dpos.ListCandidates(pctx)
	require.Nil(t, err)
	assert.Equal(t, len(candidates), 3)

	validators, err := dpos.ListValidators(pctx)
	require.Nil(t, err)
	assert.Equal(t, len(validators), 0)

	require.NoError(t, elect(pctx, dpos.Address))

	validators, err = dpos.ListValidators(pctx)
	require.Nil(t, err)
	assert.Equal(t, len(validators), 2)

	oldRewardsValue := big.NewInt(0)
	for i := 0; i < 10; i++ {
		require.NoError(t, elect(pctx, dpos.Address))
		delegations, amount, _, err := dpos.CheckDelegation(pctx.WithSender(addr1), &addr1, &addr1)
		require.NoError(t, err)
		// get rewards delegaiton which is always at index 0
		delegation := delegations[REWARD_DELEGATION_INDEX]
		assert.True(t, delegation.Amount.Value.Int.Cmp(oldRewardsValue) == 1)
		oldRewardsValue = amount
	}

	// Change WhitelistAmount and verify that it got changed correctly
	validators, err = dpos.ListValidators(pctx)
	require.Nil(t, err)
	validator := validators[0]
	assert.Equal(t, whitelistAmount, validator.WhitelistAmount.Value.Int)

	newWhitelistAmount := big.NewInt(2000000000000)

	// only oracle
	err = dpos.ChangeWhitelistInfo(pctx.WithSender(addr2), &addr1, newWhitelistAmount, nil)
	require.Error(t, err)

	err = dpos.ChangeWhitelistInfo(pctx.WithSender(addr1), &addr1, newWhitelistAmount, nil)
	require.Nil(t, err)

	validators, err = dpos.ListValidators(pctx)
	require.Nil(t, err)
	validator = validators[0]
	assert.Equal(t, newWhitelistAmount, validator.WhitelistAmount.Value.Int)
}

func TestValidatorRewards(t *testing.T) {
	// Init the coin balances
	pctx := plugin.CreateFakeContext(delegatorAddress1, diadem.Address{}).WithBlock(diadem.BlockHeader{
		ChainID: chainID,
		Time:    startTime,
	})
	coinAddr := pctx.CreateContract(coin.Contract)

	coinContract := &coin.Coin{}
	coinCtx := pctx.WithAddress(coinAddr)
	coinContract.Init(contractpb.WrapPluginContext(coinCtx), &coin.InitRequest{
		Accounts: []*coin.InitialAccount{
			makeAccount(delegatorAddress1, 100000000),
			makeAccount(delegatorAddress2, 100000000),
			makeAccount(delegatorAddress3, 100000000),
			makeAccount(addr1, 100000000),
			makeAccount(addr2, 100000000),
			makeAccount(addr3, 100000000),
		},
	})

	// create dpos contract
	dpos, err := deployDPOSContract(pctx, &Params{
		ValidatorCount:      10,
		CoinContractAddress: coinAddr.MarshalPB(),
	})
	require.Nil(t, err)

	// transfer coins to reward fund
	amount := big.NewInt(10)
	amount.Exp(amount, big.NewInt(19), nil)
	coinContract.Transfer(contractpb.WrapPluginContext(coinCtx), &coin.TransferRequest{
		To: dpos.Address.MarshalPB(),
		Amount: &types.BigUInt{
			Value: common.BigUInt{amount},
		},
	})

	registrationFee := &types.BigUInt{Value: *scientificNotation(defaultRegistrationRequirement, tokenDecimals)}

	err = coinContract.Approve(contractpb.WrapPluginContext(coinCtx.WithSender(addr1)), &coin.ApproveRequest{
		Spender: dpos.Address.MarshalPB(),
		Amount:  registrationFee,
	})
	require.Nil(t, err)

	err = dpos.RegisterCandidate(pctx.WithSender(addr1), pubKey1, nil, nil, nil, nil, nil, nil)
	require.Nil(t, err)

	err = coinContract.Approve(contractpb.WrapPluginContext(coinCtx.WithSender(addr2)), &coin.ApproveRequest{
		Spender: dpos.Address.MarshalPB(),
		Amount:  registrationFee,
	})
	require.Nil(t, err)

	err = dpos.RegisterCandidate(pctx.WithSender(addr2), pubKey2, nil, nil, nil, nil, nil, nil)
	require.Nil(t, err)

	err = coinContract.Approve(contractpb.WrapPluginContext(coinCtx.WithSender(addr3)), &coin.ApproveRequest{
		Spender: dpos.Address.MarshalPB(),
		Amount:  registrationFee,
	})
	require.Nil(t, err)

	err = dpos.RegisterCandidate(pctx.WithSender(addr3), pubKey3, nil, nil, nil, nil, nil, nil)
	require.Nil(t, err)

	candidates, err := dpos.ListCandidates(pctx)
	require.Nil(t, err)
	assert.Equal(t, len(candidates), 3)

	validators, err := dpos.ListValidators(pctx)
	require.Nil(t, err)
	assert.Equal(t, len(validators), 0)

	require.NoError(t, elect(pctx, dpos.Address))

	validators, err = dpos.ListValidators(pctx)
	require.Nil(t, err)
	assert.Equal(t, len(validators), 3)

	// Two delegators delegate 1/2 and 1/4 of a registration fee respectively
	smallDelegationAmount := diadem.NewBigUIntFromInt(0)
	smallDelegationAmount.Div(&registrationFee.Value, diadem.NewBigUIntFromInt(4))
	largeDelegationAmount := diadem.NewBigUIntFromInt(0)
	largeDelegationAmount.Div(&registrationFee.Value, diadem.NewBigUIntFromInt(2))

	err = coinContract.Approve(contractpb.WrapPluginContext(coinCtx.WithSender(delegatorAddress1)), &coin.ApproveRequest{
		Spender: dpos.Address.MarshalPB(),
		Amount:  &types.BigUInt{Value: *smallDelegationAmount},
	})
	require.Nil(t, err)

	err = dpos.Delegate(pctx.WithSender(delegatorAddress1), &addr1, smallDelegationAmount.Int, nil, nil)
	require.Nil(t, err)

	err = coinContract.Approve(contractpb.WrapPluginContext(coinCtx.WithSender(delegatorAddress2)), &coin.ApproveRequest{
		Spender: dpos.Address.MarshalPB(),
		Amount:  &types.BigUInt{Value: *largeDelegationAmount},
	})
	require.Nil(t, err)

	err = dpos.Delegate(pctx.WithSender(delegatorAddress2), &addr1, largeDelegationAmount.Int, nil, nil)
	require.Nil(t, err)

	for i := 0; i < 10000; i++ {
		require.NoError(t, elect(pctx, dpos.Address))
	}

	_, amount, _, err = dpos.CheckDelegation(pctx.WithSender(addr1), &addr1, &addr1)
	require.Nil(t, err)
	assert.Equal(t, amount.Cmp(big.NewInt(0)), 1)

	_, delegator1Claim, _, err := dpos.CheckDelegation(pctx.WithSender(delegatorAddress1), &addr1, &delegatorAddress1)
	require.Nil(t, err)
	assert.Equal(t, delegator1Claim.Cmp(big.NewInt(0)), 1)

	_, delegator2Claim, _, err := dpos.CheckDelegation(pctx.WithSender(delegatorAddress2), &addr1, &delegatorAddress2)
	require.Nil(t, err)
	assert.Equal(t, delegator2Claim.Cmp(big.NewInt(0)), 1)

	halvedDelegator2Claim := big.NewInt(0)
	halvedDelegator2Claim.Div(delegator2Claim, big.NewInt(2))
	difference := big.NewInt(0)
	difference.Sub(delegator1Claim, halvedDelegator2Claim)

	// Checking that Delegator2's claim is almost exactly half of Delegator1's claim
	maximumDifference := scientificNotation(1, tokenDecimals)
	assert.Equal(t, difference.CmpAbs(maximumDifference.Int), -1)

	// Using unbond to claim reward delegation
	err = dpos.Unbond(pctx.WithSender(addr1), &addr1, big.NewInt(0), REWARD_DELEGATION_INDEX)
	require.Nil(t, err)

	// check that addr1's balance increases after rewards claim
	balanceBeforeUnbond, err := coinContract.BalanceOf(contractpb.WrapPluginContext(coinCtx), &coin.BalanceOfRequest{
		Owner: addr1.MarshalPB(),
	})
	require.Nil(t, err)

	// allowing reward delegation to unbond
	require.NoError(t, elect(pctx, dpos.Address))
	require.Nil(t, err)

	balanceAfterUnbond, err := coinContract.BalanceOf(contractpb.WrapPluginContext(coinCtx), &coin.BalanceOfRequest{
		Owner: addr1.MarshalPB(),
	})
	require.Nil(t, err)

	assert.True(t, balanceAfterUnbond.Balance.Value.Cmp(&balanceBeforeUnbond.Balance.Value) > 0)

	// check that difference is exactly the undelegated amount

	// check current delegation amount
}

func TestReferrerRewards(t *testing.T) {
	// Init the coin balances
	pctx := plugin.CreateFakeContext(delegatorAddress1, diadem.Address{}).WithBlock(diadem.BlockHeader{
		ChainID: chainID,
		Time:    startTime,
	})
	coinAddr := pctx.CreateContract(coin.Contract)

	coinContract := &coin.Coin{}
	coinCtx := pctx.WithAddress(coinAddr)
	coinContract.Init(contractpb.WrapPluginContext(coinCtx), &coin.InitRequest{
		Accounts: []*coin.InitialAccount{
			makeAccount(delegatorAddress1, 100000000),
			makeAccount(delegatorAddress2, 100000000),
			makeAccount(delegatorAddress3, 100000000),
			makeAccount(addr1, 100000000),
		},
	})

	// create dpos contract
	dpos, err := deployDPOSContract(pctx, &Params{
		ValidatorCount:      10,
		CoinContractAddress: coinAddr.MarshalPB(),
		OracleAddress:       addr1.MarshalPB(),
	})
	require.Nil(t, err)

	registrationFee := &types.BigUInt{Value: *scientificNotation(defaultRegistrationRequirement, tokenDecimals)}

	err = coinContract.Approve(contractpb.WrapPluginContext(coinCtx.WithSender(addr1)), &coin.ApproveRequest{
		Spender: dpos.Address.MarshalPB(),
		Amount:  registrationFee,
	})
	require.Nil(t, err)

	fee := uint64(2000)
	pct := uint64(10000)
	err = dpos.RegisterCandidate(pctx.WithSender(addr1), pubKey1, nil, &fee, &pct, nil, nil, nil)
	require.Nil(t, err)

	candidates, err := dpos.ListCandidates(pctx)
	require.Nil(t, err)
	assert.Equal(t, len(candidates), 1)

	validators, err := dpos.ListValidators(pctx)
	require.Nil(t, err)
	assert.Equal(t, len(validators), 0)

	require.NoError(t, elect(pctx, dpos.Address))

	validators, err = dpos.ListValidators(pctx)
	require.Nil(t, err)
	assert.Equal(t, len(validators), 1)

	del1Name := "del1"
	// Register two referrers
	err = dpos.RegisterReferrer(pctx.WithSender(addr1), delegatorAddress1, "del1")
	require.Nil(t, err)

	err = dpos.RegisterReferrer(pctx.WithSender(addr1), delegatorAddress2, "del2")
	require.Nil(t, err)

	delegationAmount := big.NewInt(1e18)
	err = coinContract.Approve(contractpb.WrapPluginContext(coinCtx.WithSender(delegatorAddress3)), &coin.ApproveRequest{
		Spender: dpos.Address.MarshalPB(),
		Amount:  &types.BigUInt{Value: *diadem.NewBigUInt(delegationAmount)},
	})
	require.Nil(t, err)

	err = dpos.Delegate(pctx.WithSender(delegatorAddress3), &addr1, delegationAmount, nil, &del1Name)
	require.Nil(t, err)

	for i := 0; i < 10; i++ {
		require.NoError(t, elect(pctx, dpos.Address))
	}

	_, amount, _, err := dpos.CheckDelegation(pctx.WithSender(addr1), &limboValidatorAddress, &delegatorAddress1)
	require.Nil(t, err)
	assert.Equal(t, amount.Cmp(big.NewInt(0)), 1)
}

func TestRewardTiers(t *testing.T) {
	// Init the coin balances
	pctx := plugin.CreateFakeContext(delegatorAddress1, diadem.Address{}).WithBlock(diadem.BlockHeader{
		ChainID: chainID,
		Time:    startTime,
	})
	coinAddr := pctx.CreateContract(coin.Contract)

	coinContract := &coin.Coin{}
	coinCtx := pctx.WithAddress(coinAddr)
	coinContract.Init(contractpb.WrapPluginContext(coinCtx), &coin.InitRequest{
		Accounts: []*coin.InitialAccount{
			makeAccount(delegatorAddress1, 100000000),
			makeAccount(delegatorAddress2, 100000000),
			makeAccount(delegatorAddress3, 100000000),
			makeAccount(delegatorAddress4, 100000000),
			makeAccount(delegatorAddress5, 100000000),
			makeAccount(delegatorAddress6, 100000000),
			makeAccount(addr1, 100000000),
			makeAccount(addr2, 100000000),
			makeAccount(addr3, 100000000),
		},
	})

	// Init the dpos contract
	dpos, err := deployDPOSContract(pctx, &Params{
		ValidatorCount:      10,
		CoinContractAddress: coinAddr.MarshalPB(),
	})
	require.Nil(t, err)

	// transfer coins to reward fund
	amount := big.NewInt(10)
	amount.Exp(amount, big.NewInt(19), nil)
	coinContract.Transfer(contractpb.WrapPluginContext(coinCtx), &coin.TransferRequest{
		To: dpos.Address.MarshalPB(),
		Amount: &types.BigUInt{
			Value: common.BigUInt{amount},
		},
	})

	registrationFee := &types.BigUInt{Value: *scientificNotation(defaultRegistrationRequirement, tokenDecimals)}

	err = coinContract.Approve(contractpb.WrapPluginContext(coinCtx.WithSender(addr1)), &coin.ApproveRequest{
		Spender: dpos.Address.MarshalPB(),
		Amount:  registrationFee,
	})
	require.Nil(t, err)

	err = dpos.RegisterCandidate(pctx.WithSender(addr1), pubKey1, nil, nil, nil, nil, nil, nil)
	require.Nil(t, err)

	err = coinContract.Approve(contractpb.WrapPluginContext(coinCtx.WithSender(addr2)), &coin.ApproveRequest{
		Spender: dpos.Address.MarshalPB(),
		Amount:  registrationFee,
	})
	require.Nil(t, err)

	err = dpos.RegisterCandidate(pctx.WithSender(addr2), pubKey2, nil, nil, nil, nil, nil, nil)
	require.Nil(t, err)

	err = coinContract.Approve(contractpb.WrapPluginContext(coinCtx.WithSender(addr3)), &coin.ApproveRequest{
		Spender: dpos.Address.MarshalPB(),
		Amount:  registrationFee,
	})
	require.Nil(t, err)

	err = dpos.RegisterCandidate(pctx.WithSender(addr3), pubKey3, nil, nil, nil, nil, nil, nil)
	require.Nil(t, err)

	candidates, err := dpos.ListCandidates(pctx)
	require.Nil(t, err)
	assert.Equal(t, len(candidates), 3)

	validators, err := dpos.ListValidators(pctx)
	require.Nil(t, err)
	assert.Equal(t, len(validators), 0)

	require.NoError(t, elect(pctx, dpos.Address))

	validators, err = dpos.ListValidators(pctx)
	require.Nil(t, err)
	assert.Equal(t, len(validators), 3)

	// tinyDelegationAmount = one DIADEM token
	tinyDelegationAmount := scientificNotation(1, tokenDecimals)
	smallDelegationAmount := diadem.NewBigUIntFromInt(0)
	smallDelegationAmount.Div(&registrationFee.Value, diadem.NewBigUIntFromInt(4))
	largeDelegationAmount := diadem.NewBigUIntFromInt(0)
	largeDelegationAmount.Div(&registrationFee.Value, diadem.NewBigUIntFromInt(2))

	err = coinContract.Approve(contractpb.WrapPluginContext(coinCtx.WithSender(delegatorAddress1)), &coin.ApproveRequest{
		Spender: dpos.Address.MarshalPB(),
		Amount:  &types.BigUInt{Value: *smallDelegationAmount},
	})
	require.Nil(t, err)

	// LocktimeTier should default to 0 for delegatorAddress1
	err = dpos.Delegate(pctx.WithSender(delegatorAddress1), &addr1, smallDelegationAmount.Int, nil, nil)
	require.Nil(t, err)

	err = coinContract.Approve(contractpb.WrapPluginContext(coinCtx.WithSender(delegatorAddress2)), &coin.ApproveRequest{
		Spender: dpos.Address.MarshalPB(),
		Amount:  &types.BigUInt{Value: *smallDelegationAmount},
	})
	require.Nil(t, err)

	tier := uint64(2)
	err = dpos.Delegate(pctx.WithSender(delegatorAddress2), &addr1, smallDelegationAmount.Int, &tier, nil)
	require.Nil(t, err)

	err = coinContract.Approve(contractpb.WrapPluginContext(coinCtx.WithSender(delegatorAddress3)), &coin.ApproveRequest{
		Spender: dpos.Address.MarshalPB(),
		Amount:  &types.BigUInt{Value: *smallDelegationAmount},
	})
	require.Nil(t, err)

	tier = uint64(3)
	err = dpos.Delegate(pctx.WithSender(delegatorAddress3), &addr1, smallDelegationAmount.Int, &tier, nil)
	require.Nil(t, err)

	err = coinContract.Approve(contractpb.WrapPluginContext(coinCtx.WithSender(delegatorAddress4)), &coin.ApproveRequest{
		Spender: dpos.Address.MarshalPB(),
		Amount:  &types.BigUInt{Value: *smallDelegationAmount},
	})
	require.Nil(t, err)

	tier = uint64(1)
	err = dpos.Delegate(pctx.WithSender(delegatorAddress4), &addr1, smallDelegationAmount.Int, &tier, nil)
	require.Nil(t, err)

	err = coinContract.Approve(contractpb.WrapPluginContext(coinCtx.WithSender(delegatorAddress5)), &coin.ApproveRequest{
		Spender: dpos.Address.MarshalPB(),
		Amount:  &types.BigUInt{Value: *largeDelegationAmount},
	})
	require.Nil(t, err)

	// Though Delegator5 delegates to addr2 and not addr1 like the rest of the
	// delegators, he should still receive the same rewards proportional to his
	// delegation parameters
	tier = uint64(2)
	err = dpos.Delegate(pctx.WithSender(delegatorAddress5), &addr2, largeDelegationAmount.Int, &tier, nil)
	require.Nil(t, err)

	err = coinContract.Approve(contractpb.WrapPluginContext(coinCtx.WithSender(delegatorAddress6)), &coin.ApproveRequest{
		Spender: dpos.Address.MarshalPB(),
		Amount:  &types.BigUInt{Value: *tinyDelegationAmount},
	})
	require.Nil(t, err)

	// by delegating a very small amount, delegator6 demonstrates that
	// delegators can contribute far less than 0.01% of a validator's total
	// delegation and still be rewarded
	err = dpos.Delegate(pctx.WithSender(delegatorAddress6), &addr1, tinyDelegationAmount.Int, nil, nil)
	require.Nil(t, err)

	for i := 0; i < 10000; i++ {
		require.NoError(t, elect(pctx, dpos.Address))
	}

	addr1Claim, err := dpos.CheckRewardDelegation(pctx.WithSender(addr1), &addr1)
	require.Nil(t, err)
	assert.Equal(t, addr1Claim.Amount.Value.Cmp(common.BigZero()), 1)

	delegator1Claim, err := dpos.CheckRewardDelegation(pctx.WithSender(delegatorAddress1), &addr1)
	require.Nil(t, err)
	assert.Equal(t, delegator1Claim.Amount.Value.Cmp(common.BigZero()), 1)

	delegator2Claim, err := dpos.CheckRewardDelegation(pctx.WithSender(delegatorAddress2), &addr1)
	require.Nil(t, err)
	assert.Equal(t, delegator2Claim.Amount.Value.Cmp(common.BigZero()), 1)

	delegator3Claim, err := dpos.CheckRewardDelegation(pctx.WithSender(delegatorAddress3), &addr1)
	require.Nil(t, err)
	assert.Equal(t, delegator3Claim.Amount.Value.Cmp(common.BigZero()), 1)

	delegator4Claim, err := dpos.CheckRewardDelegation(pctx.WithSender(delegatorAddress4), &addr1)
	require.Nil(t, err)
	assert.Equal(t, delegator4Claim.Amount.Value.Cmp(common.BigZero()), 1)

	delegator5Claim, err := dpos.CheckRewardDelegation(pctx.WithSender(delegatorAddress5), &addr2)
	require.Nil(t, err)
	assert.Equal(t, delegator5Claim.Amount.Value.Cmp(common.BigZero()), 1)

	delegator6Claim, err := dpos.CheckRewardDelegation(pctx.WithSender(delegatorAddress6), &addr1)
	require.Nil(t, err)
	assert.Equal(t, delegator6Claim.Amount.Value.Cmp(common.BigZero()), 1)

	maximumDifference := scientificNotation(1, tokenDecimals)
	difference := diadem.NewBigUIntFromInt(0)

	// Checking that Delegator2's claim is almost exactly twice Delegator1's claim
	scaledDelegator1Claim := CalculateFraction(*diadem.NewBigUIntFromInt(20000), delegator1Claim.Amount.Value)
	difference.Sub(&scaledDelegator1Claim, &delegator2Claim.Amount.Value)
	assert.Equal(t, difference.Int.CmpAbs(maximumDifference.Int), -1)

	// Checking that Delegator3's & Delegator5's claim is almost exactly four times Delegator1's claim
	scaledDelegator1Claim = CalculateFraction(*diadem.NewBigUIntFromInt(40000), delegator1Claim.Amount.Value)

	difference.Sub(&scaledDelegator1Claim, &delegator3Claim.Amount.Value)
	assert.Equal(t, difference.Int.CmpAbs(maximumDifference.Int), -1)

	difference.Sub(&scaledDelegator1Claim, &delegator5Claim.Amount.Value)
	assert.Equal(t, difference.Int.CmpAbs(maximumDifference.Int), -1)

	// Checking that Delegator4's claim is almost exactly 1.5 times Delegator1's claim
	scaledDelegator1Claim = CalculateFraction(*diadem.NewBigUIntFromInt(15000), delegator1Claim.Amount.Value)
	difference.Sub(&scaledDelegator1Claim, &delegator4Claim.Amount.Value)
	assert.Equal(t, difference.Int.CmpAbs(maximumDifference.Int), -1)

	// Testing total delegation functionality

	_, amount, weightedAmount, err := dpos.CheckAllDelegations(pctx, &delegatorAddress3)
	require.Nil(t, err)
	assert.True(t, amount.Cmp(smallDelegationAmount.Int) > 0)
	expectedWeightedAmount := CalculateFraction(*diadem.NewBigUIntFromInt(40000), *smallDelegationAmount)
	assert.True(t, weightedAmount.Cmp(expectedWeightedAmount.Int) > 0)
}

// Besides reward cap functionality, this also demostrates 0-fee candidate registration
func TestRewardCap(t *testing.T) {
	// Init the coin balances
	pctx := plugin.CreateFakeContext(delegatorAddress1, diadem.Address{}).WithBlock(diadem.BlockHeader{
		ChainID: chainID,
		Time:    startTime,
	})
	coinAddr := pctx.CreateContract(coin.Contract)

	coinContract := &coin.Coin{}
	coinCtx := pctx.WithAddress(coinAddr)
	coinContract.Init(contractpb.WrapPluginContext(coinCtx), &coin.InitRequest{
		Accounts: []*coin.InitialAccount{
			makeAccount(delegatorAddress1, 100000000),
			makeAccount(delegatorAddress2, 100000000),
			makeAccount(delegatorAddress3, 100000000),
			makeAccount(addr1, 100000000),
			makeAccount(addr2, 100000000),
			makeAccount(addr3, 100000000),
		},
	})

	// Init the dpos contract
	maxReward := scientificNotation(100, tokenDecimals)
	dpos, err := deployDPOSContract(pctx, &Params{
		ValidatorCount:          10,
		CoinContractAddress:     coinAddr.MarshalPB(),
		MaxYearlyReward:         &types.BigUInt{Value: *maxReward},
		RegistrationRequirement: &types.BigUInt{Value: *diadem.NewBigUIntFromInt(0)},
	})
	require.Nil(t, err)

	// transfer coins to reward fund
	amount := big.NewInt(10)
	amount.Exp(amount, big.NewInt(19), nil)
	coinContract.Transfer(contractpb.WrapPluginContext(coinCtx), &coin.TransferRequest{
		To: dpos.Address.MarshalPB(),
		Amount: &types.BigUInt{
			Value: common.BigUInt{amount},
		},
	})

	err = dpos.RegisterCandidate(pctx.WithSender(addr1), pubKey1, nil, nil, nil, nil, nil, nil)
	require.Nil(t, err)

	err = dpos.RegisterCandidate(pctx.WithSender(addr2), pubKey2, nil, nil, nil, nil, nil, nil)
	require.Nil(t, err)

	err = dpos.RegisterCandidate(pctx.WithSender(addr3), pubKey3, nil, nil, nil, nil, nil, nil)
	require.Nil(t, err)

	candidates, err := dpos.ListCandidates(pctx)
	require.Nil(t, err)
	assert.Equal(t, len(candidates), 3)

	validators, err := dpos.ListValidators(pctx)
	require.Nil(t, err)
	assert.Equal(t, len(validators), 0)

	require.NoError(t, elect(pctx, dpos.Address))

	// Validators are still 0 because they have no stake delegated
	// and they registered with 0
	validators, err = dpos.ListValidators(pctx)
	require.Nil(t, err)
	assert.Equal(t, len(validators), 0)

	delegationAmount := scientificNotation(1000, tokenDecimals)
	err = coinContract.Approve(contractpb.WrapPluginContext(coinCtx.WithSender(delegatorAddress1)), &coin.ApproveRequest{
		Spender: dpos.Address.MarshalPB(),
		Amount:  &types.BigUInt{Value: *delegationAmount},
	})
	require.Nil(t, err)

	err = dpos.Delegate(pctx.WithSender(delegatorAddress1), &addr1, delegationAmount.Int, nil, nil)
	require.Nil(t, err)

	err = coinContract.Approve(contractpb.WrapPluginContext(coinCtx.WithSender(delegatorAddress2)), &coin.ApproveRequest{
		Spender: dpos.Address.MarshalPB(),
		Amount:  &types.BigUInt{Value: *delegationAmount},
	})
	require.Nil(t, err)

	err = dpos.Delegate(pctx.WithSender(delegatorAddress2), &addr2, delegationAmount.Int, nil, nil)
	require.Nil(t, err)

	// With a default yearly reward of 5% of one's token holdings, the two
	// delegators should reach their rewards limits by both delegating exactly
	// 1000, or 2000 combined since 2000 = 100 (the max yearly reward) / 0.05
	require.NoError(t, elect(pctx, dpos.Address))

	// 2 validators have non-0 stake so they are elected now (3rd still has 0)
	validators, err = dpos.ListValidators(pctx)
	require.Nil(t, err)
	assert.Equal(t, len(validators), 2)

	require.NoError(t, elect(pctx, dpos.Address))

	delegator1Claim, err := dpos.CheckRewardDelegation(pctx.WithSender(delegatorAddress1), &addr1)
	require.Nil(t, err)
	assert.Equal(t, delegator1Claim.Amount.Value.Cmp(&diadem.BigUInt{big.NewInt(0)}), 1)

	delegator2Claim, err := dpos.CheckRewardDelegation(pctx.WithSender(delegatorAddress2), &addr2)
	require.Nil(t, err)
	assert.Equal(t, delegator2Claim.Amount.Value.Cmp(&diadem.BigUInt{big.NewInt(0)}), 1)

	//                           |---- this 2 is the election cycle length used when,
	//    v--- delegationAmount  v     for testing, a 0-sec election time is set
	// ((1000 * 10**18) * 0.05 * 2) / (365 * 24 * 3600) = 3.1709791983764585e12
	expectedAmount := diadem.NewBigUIntFromInt(3170979198376)
	assert.Equal(t, *expectedAmount, delegator2Claim.Amount.Value)

	err = coinContract.Approve(contractpb.WrapPluginContext(coinCtx.WithSender(delegatorAddress3)), &coin.ApproveRequest{
		Spender: dpos.Address.MarshalPB(),
		Amount:  &types.BigUInt{Value: *delegationAmount},
	})
	require.Nil(t, err)

	err = dpos.Delegate(pctx.WithSender(delegatorAddress3), &addr1, delegationAmount.Int, nil, nil)
	require.Nil(t, err)

	// run one election to get Delegator3 elected as a validator
	require.NoError(t, elect(pctx, dpos.Address))

	// run another election to get Delegator3 his first reward distribution
	require.NoError(t, elect(pctx, dpos.Address))

	delegator3Claim, err := dpos.CheckRewardDelegation(pctx.WithSender(delegatorAddress3), &addr1)
	require.Nil(t, err)
	assert.Equal(t, delegator3Claim.Amount.Value.Cmp(&diadem.BigUInt{big.NewInt(0)}), 1)

	// verifiying that claim is smaller than what was given when delegations
	// were smaller and below max yearly reward cap.
	// delegator3Claim should be ~2/3 of delegator2Claim
	assert.Equal(t, delegator2Claim.Amount.Value.Cmp(&delegator3Claim.Amount.Value), 1)
	scaledDelegator3Claim := CalculateFraction(*diadem.NewBigUIntFromInt(15000), delegator3Claim.Amount.Value)
	difference := common.BigZero()
	difference.Sub(&scaledDelegator3Claim, &delegator2Claim.Amount.Value)
	// amounts must be within 7 * 10^-10 tokens of each other to be correct
	maximumDifference := diadem.NewBigUIntFromInt(700000000)
	assert.Equal(t, difference.Int.CmpAbs(maximumDifference.Int), -1)
}

func TestMultiDelegate(t *testing.T) {
	pctx := plugin.CreateFakeContext(addr1, addr1)

	// Deploy the coin contract (DPOS Init() will attempt to resolve it)
	coinContract := &coin.Coin{}
	coinAddr := pctx.CreateContract(coin.Contract)
	coinCtx := pctx.WithAddress(coinAddr)
	coinContract.Init(contractpb.WrapPluginContext(coinCtx), &coin.InitRequest{
		Accounts: []*coin.InitialAccount{
			makeAccount(delegatorAddress1, 1000000000000000000),
			makeAccount(addr1, 1000000000000000000),
		},
	})

	dpos, err := deployDPOSContract(pctx, &Params{
		ValidatorCount:          21,
		RegistrationRequirement: &types.BigUInt{Value: *diadem.NewBigUIntFromInt(0)},
	})
	require.Nil(t, err)

	err = dpos.RegisterCandidate(pctx.WithSender(addr1), pubKey1, nil, nil, nil, nil, nil, nil)
	require.Nil(t, err)

	delegationAmount := &types.BigUInt{Value: diadem.BigUInt{big.NewInt(2000)}}
	numberOfDelegations := int64(200)

	for i := uint64(0); i < uint64(numberOfDelegations); i++ {
		err = coinContract.Approve(contractpb.WrapPluginContext(coinCtx.WithSender(addr1)), &coin.ApproveRequest{
			Spender: dpos.Address.MarshalPB(),
			Amount:  delegationAmount,
		})
		require.Nil(t, err)

		tier := uint64(i % 4) // testing delegations with a variety of locktime tiers
		err = dpos.Delegate(pctx.WithSender(addr1), &addr1, delegationAmount.Value.Int, &tier, nil)
		require.Nil(t, err)

		require.NoError(t, elect(pctx, dpos.Address))
	}

	delegations, amount, _, err := dpos.CheckDelegation(pctx.WithSender(addr1), &addr1, &addr1)
	require.Nil(t, err)

	expectedAmount := big.NewInt(0)
	expectedAmount = expectedAmount.Mul(delegationAmount.Value.Int, big.NewInt(numberOfDelegations))
	assert.True(t, amount.Cmp(expectedAmount) == 0)

	// we add one to account for the rewards delegation
	assert.True(t, len(delegations) == int(numberOfDelegations+1))

	numDelegations := DelegationsCount(contractpb.WrapPluginContext(pctx.WithAddress(dpos.Address)))
	assert.Equal(t, numDelegations, 201)

	for i := uint64(0); i < uint64(numberOfDelegations); i++ {
		err = coinContract.Approve(contractpb.WrapPluginContext(coinCtx.WithSender(delegatorAddress1)), &coin.ApproveRequest{
			Spender: dpos.Address.MarshalPB(),
			Amount:  delegationAmount,
		})
		require.Nil(t, err)

		tier := uint64(i % 4) // testing delegations with a variety of locktime tiers
		err = dpos.Delegate(pctx.WithSender(delegatorAddress1), &addr1, delegationAmount.Value.Int, &tier, nil)
		require.Nil(t, err)

		require.NoError(t, elect(pctx, dpos.Address))
	}

	delegations, amount, _, err = dpos.CheckDelegation(pctx.WithSender(addr1), &addr1, &delegatorAddress1)
	require.Nil(t, err)
	assert.True(t, amount.Cmp(expectedAmount) == 0)
	assert.True(t, len(delegations) == int(numberOfDelegations+1))

	numDelegations = DelegationsCount(contractpb.WrapPluginContext(pctx.WithAddress(dpos.Address)))
	assert.Equal(t, numDelegations, 402)

	// advance contract time enough to unlock all delegations
	now := uint64(pctx.Now().Unix())
	pctx.SetTime(pctx.Now().Add(time.Duration(now+TierLocktimeMap[3]+1) * time.Second))

	err = dpos.Unbond(pctx.WithSender(addr1), &addr1, delegationAmount.Value.Int, 100)
	require.Nil(t, err)

	require.NoError(t, elect(pctx, dpos.Address))

	numDelegations = DelegationsCount(contractpb.WrapPluginContext(pctx.WithAddress(dpos.Address)))
	assert.Equal(t, numDelegations, 402-1)

	// Check that all delegations have had their tier reset to TIER_ZERO
	listAllDelegations, err := dpos.ListAllDelegations(pctx)
	require.Nil(t, err)

	for _, listDelegationsResponse := range listAllDelegations {
		for _, delegation := range listDelegationsResponse.Delegations {
			assert.Equal(t, delegation.LocktimeTier, TIER_ZERO)
		}
	}
}

func TestLockup(t *testing.T) {
	pctx := plugin.CreateFakeContext(addr1, addr1)

	// Deploy the coin contract (DPOS Init() will attempt to resolve it)
	coinContract := &coin.Coin{}
	coinAddr := pctx.CreateContract(coin.Contract)
	coinCtx := pctx.WithAddress(coinAddr)
	coinContract.Init(contractpb.WrapPluginContext(coinCtx), &coin.InitRequest{
		Accounts: []*coin.InitialAccount{
			makeAccount(addr1, 1000000000000000000),
			makeAccount(delegatorAddress1, 1000000000000000000),
			makeAccount(delegatorAddress2, 1000000000000000000),
			makeAccount(delegatorAddress3, 1000000000000000000),
			makeAccount(delegatorAddress4, 1000000000000000000),
		},
	})

	dpos, err := deployDPOSContract(pctx, &Params{
		ValidatorCount:          21,
		RegistrationRequirement: &types.BigUInt{Value: *diadem.NewBigUIntFromInt(0)},
	})
	require.Nil(t, err)

	err = dpos.RegisterCandidate(pctx.WithSender(addr1), pubKey1, nil, nil, nil, nil, nil, nil)
	require.Nil(t, err)

	now := uint64(pctx.Now().Unix())
	delegationAmount := &types.BigUInt{Value: diadem.BigUInt{big.NewInt(2000)}}

	var tests = []struct {
		Delegator diadem.Address
		Tier      uint64
	}{
		{delegatorAddress1, 0},
		{delegatorAddress2, 1},
		{delegatorAddress3, 2},
		{delegatorAddress4, 3},
	}

	for _, test := range tests {
		expectedLockup := now + TierLocktimeMap[LocktimeTier(test.Tier)]

		// delegating
		err = coinContract.Approve(contractpb.WrapPluginContext(coinCtx.WithSender(test.Delegator)), &coin.ApproveRequest{
			Spender: dpos.Address.MarshalPB(),
			Amount:  delegationAmount,
		})
		require.Nil(t, err)

		err = dpos.Delegate(pctx.WithSender(test.Delegator), &addr1, delegationAmount.Value.Int, &test.Tier, nil)
		require.Nil(t, err)

		// checking delegation pre-election
		delegations, _, _, err := dpos.CheckDelegation(pctx.WithSender(addr1), &addr1, &test.Delegator)
		require.Nil(t, err)
		delegation := delegations[len(delegations)-1]

		assert.Equal(t, expectedLockup, delegation.LockTime)
		assert.Equal(t, true, uint64(delegation.LocktimeTier) == test.Tier)
		assert.Equal(t, delegation.Amount.Value.Cmp(common.BigZero()), 0)
		assert.Equal(t, delegation.UpdateAmount.Value.Cmp(&delegationAmount.Value), 0)

		// running election
		require.NoError(t, elect(pctx, dpos.Address))

		// checking delegation post-election
		delegations, _, _, err = dpos.CheckDelegation(pctx.WithSender(addr1), &addr1, &test.Delegator)
		require.Nil(t, err)
		delegation = delegations[len(delegations)-1]

		assert.Equal(t, expectedLockup, delegation.LockTime)
		assert.Equal(t, true, uint64(delegation.LocktimeTier) == test.Tier)
		assert.Equal(t, delegation.UpdateAmount.Value.Cmp(common.BigZero()), 0)
		assert.Equal(t, delegation.Amount.Value.Cmp(&delegationAmount.Value), 0)
	}

	// setting time to reset tiers of all delegations except the last
	pctx.SetTime(pctx.Now().Add(time.Duration(now+TierLocktimeMap[2]+1) * time.Second))

	// running election to trigger locktime resets
	require.NoError(t, elect(pctx, dpos.Address))

	delegations, _, _, err := dpos.CheckDelegation(pctx.WithSender(addr1), &addr1, &delegatorAddress3)
	require.Nil(t, err)
	assert.Equal(t, TIER_ZERO, delegations[len(delegations)-1].LocktimeTier)

	delegations, _, _, err = dpos.CheckDelegation(pctx.WithSender(addr1), &addr1, &delegatorAddress4)
	require.Nil(t, err)
	assert.Equal(t, TIER_THREE, delegations[len(delegations)-1].LocktimeTier)
}

func TestApplyPowerCap(t *testing.T) {
	var tests = []struct {
		input  []*Validator
		output []*Validator
	}{
		{
			[]*Validator{&Validator{Power: 10}},
			[]*Validator{&Validator{Power: 10}},
		},
		{
			[]*Validator{&Validator{Power: 10}, &Validator{Power: 1}},
			[]*Validator{&Validator{Power: 10}, &Validator{Power: 1}},
		},
		{
			[]*Validator{&Validator{Power: 30}, &Validator{Power: 30}, &Validator{Power: 30}, &Validator{Power: 30}},
			[]*Validator{&Validator{Power: 30}, &Validator{Power: 30}, &Validator{Power: 30}, &Validator{Power: 30}},
		},
		{
			[]*Validator{&Validator{Power: 33}, &Validator{Power: 30}, &Validator{Power: 22}, &Validator{Power: 22}},
			[]*Validator{&Validator{Power: 29}, &Validator{Power: 29}, &Validator{Power: 24}, &Validator{Power: 24}},
		},
		{
			[]*Validator{&Validator{Power: 100}, &Validator{Power: 20}, &Validator{Power: 5}, &Validator{Power: 5}, &Validator{Power: 5}},
			[]*Validator{&Validator{Power: 37}, &Validator{Power: 35}, &Validator{Power: 20}, &Validator{Power: 20}, &Validator{Power: 20}},
		},
		{
			[]*Validator{&Validator{Power: 150}, &Validator{Power: 100}, &Validator{Power: 77}, &Validator{Power: 15}, &Validator{Power: 15}, &Validator{Power: 10}},
			[]*Validator{&Validator{Power: 102}, &Validator{Power: 102}, &Validator{Power: 86}, &Validator{Power: 24}, &Validator{Power: 24}, &Validator{Power: 19}},
		},
	}
	for _, test := range tests {
		output := applyPowerCap(test.input)
		for i, o := range output {
			assert.Equal(t, test.output[i].Power, o.Power)
		}
	}
}

// UTILITIES

func makeAccount(owner diadem.Address, bal uint64) *coin.InitialAccount {
	return &coin.InitialAccount{
		Owner:   owner.MarshalPB(),
		Balance: bal,
	}
}

func elect(pctx *plugin.FakeContext, dposAddress diadem.Address) error {
	return Elect(contractpb.WrapPluginContext(pctx.WithAddress(dposAddress)))
}
