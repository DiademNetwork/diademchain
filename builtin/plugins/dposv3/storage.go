package dposv3

import (
	"bytes"
	"fmt"
	"sort"

	diadem "github.com/diademnetwork/go-diadem"
	dtypes "github.com/diademnetwork/go-diadem/builtin/types/dposv3"
	"github.com/diademnetwork/go-diadem/common"
	contract "github.com/diademnetwork/go-diadem/plugin/contractpb"
	types "github.com/diademnetwork/go-diadem/types"
)

const (
	REWARD_DELEGATION_INDEX = 0
	DELEGATION_START_INDEX  = 1
)

var (
	stateKey       = []byte("state")
	candidatesKey  = []byte("candidates")
	delegationsKey = []byte("delegation")
	statisticsKey  = []byte("statistic")
	referrersKey   = []byte("referrers")

	requestBatchTallyKey = []byte("request_batch_tally")
)

func sortValidators(validators []*Validator) []*Validator {
	sort.Sort(byPubkey(validators))
	return validators
}

func sortCandidates(cands CandidateList) CandidateList {
	sort.Sort(byAddress(cands))
	return cands
}

func sortDelegations(delegations DelegationList) DelegationList {
	sort.Sort(byValidatorAndDelegator(delegations))
	return delegations
}

type byPubkey []*Validator

func (s byPubkey) Len() int {
	return len(s)
}

func (s byPubkey) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}

func (s byPubkey) Less(i, j int) bool {
	return bytes.Compare(s[i].PubKey, s[j].PubKey) < 0
}

type DelegationList []*DelegationIndex

func computeDelegationsKey(index uint64, validator, delegator types.Address) ([]byte, error) {
	indexBytes := []byte(fmt.Sprintf("%d", index))
	validatorAddressBytes, err := validator.Local.Marshal()
	if err != nil {
		return nil, err
	}
	delegatorAddressBytes, err := delegator.Local.Marshal()
	if err != nil {
		return nil, err
	}

	delegationKey := append(append(indexBytes, validatorAddressBytes...), delegatorAddressBytes...)
	return delegationKey, nil
}

func GetDelegation(ctx contract.StaticContext, index uint64, validator types.Address, delegator types.Address) (*Delegation, error) {
	delegationKey, err := computeDelegationsKey(index, validator, delegator)
	if err != nil {
		return nil, err
	}

	var delegation Delegation
	err = ctx.Get(append(delegationsKey, delegationKey...), &delegation)
	if err != nil {
		return nil, err
	}

	return &delegation, nil
}

// Iterates over non-rewards delegaton indices to find the next available slot
// for a new delegation entry
func GetNextDelegationIndex(ctx contract.StaticContext, validator types.Address, delegator types.Address) (uint64, error) {
	var index uint64 = DELEGATION_START_INDEX
	for {
		delegation, err := GetDelegation(ctx, index, validator, delegator)
		if err != nil && err != contract.ErrNotFound {
			return 0, err
		}

		if delegation == nil {
			break
		}
		index++
	}
	return index, nil
}

func DelegationsCount(ctx contract.StaticContext) int {
	delegations, err := loadDelegationList(ctx)
	if err != nil {
		return 0
	}

	return len(delegations)
}

func SetDelegation(ctx contract.Context, delegation *Delegation) error {
	delegations, err := loadDelegationList(ctx)
	if err != nil {
		return err
	}

	delegationIndex := &DelegationIndex{
		Validator: delegation.Validator,
		Delegator: delegation.Delegator,
		Index:     delegation.Index,
	}

	pastvalue, _ := GetDelegation(ctx, delegation.Index, *delegation.Validator, *delegation.Delegator)
	if pastvalue == nil {
		delegations = append(delegations, delegationIndex)
		if err := saveDelegationList(ctx, delegations); err != nil {
			return err
		}
	}

	delegationKey, err := computeDelegationsKey(delegationIndex.Index, *delegation.Validator, *delegation.Delegator)
	if err != nil {
		return err
	}

	return ctx.Set(append(delegationsKey, delegationKey...), delegation)
}

func DeleteDelegation(ctx contract.Context, delegation *Delegation) error {
	delegations, err := loadDelegationList(ctx)
	if err != nil {
		return err
	}

	for i, d := range delegations {
		if delegation.Validator.Local.Compare(d.Validator.Local) == 0 && delegation.Delegator.Local.Compare(d.Delegator.Local) == 0 {
			copy(delegations[i:], delegations[i+1:])
			delegations = delegations[:len(delegations)-1]
			break
		}
	}
	if err := saveDelegationList(ctx, delegations); err != nil {
		return err
	}

	delegationKey, err := computeDelegationsKey(delegation.Index, *delegation.Validator, *delegation.Delegator)
	if err != nil {
		return err
	}

	ctx.Delete(append(delegationsKey, delegationKey...))

	return nil
}

func saveDelegationList(ctx contract.Context, dl DelegationList) error {
	sorted := sortDelegations(dl)
	return ctx.Set(delegationsKey, &dtypes.DelegationList{Delegations: sorted})
}

func loadDelegationList(ctx contract.StaticContext) (DelegationList, error) {
	var pbcl dtypes.DelegationList
	err := ctx.Get(delegationsKey, &pbcl)
	if err == contract.ErrNotFound {
		return DelegationList{}, nil
	}
	if err != nil {
		return nil, err
	}
	return pbcl.Delegations, nil
}

type byValidatorAndDelegator DelegationList

func (s byValidatorAndDelegator) Len() int {
	return len(s)
}

func (s byValidatorAndDelegator) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}

func (s byValidatorAndDelegator) Less(i, j int) bool {
	vAddr1 := diadem.UnmarshalAddressPB(s[i].Validator)
	vAddr2 := diadem.UnmarshalAddressPB(s[j].Validator)
	diff := vAddr1.Compare(vAddr2)

	if diff == 0 {
		dAddr1 := diadem.UnmarshalAddressPB(s[i].Delegator)
		dAddr2 := diadem.UnmarshalAddressPB(s[j].Delegator)
		diff = dAddr1.Compare(dAddr2)
	}

	if diff == 0 {
		return s[i].Index < s[j].Index
	}

	return diff < 0
}

func GetStatistic(ctx contract.StaticContext, address diadem.Address) (*ValidatorStatistic, error) {
	addressBytes, err := address.Local.Marshal()
	if err != nil {
		return nil, err
	}
	return GetStatisticByAddressBytes(ctx, addressBytes)
}

func GetStatisticByAddressBytes(ctx contract.StaticContext, addressBytes []byte) (*ValidatorStatistic, error) {
	var statistic ValidatorStatistic
	err := ctx.Get(append(statisticsKey, addressBytes...), &statistic)
	if err != nil {
		return nil, err
	}

	return &statistic, nil
}

func SetStatistic(ctx contract.Context, statistic *ValidatorStatistic) error {
	addressBytes, err := statistic.Address.Local.Marshal()
	if err != nil {
		return err
	}

	return ctx.Set(append(statisticsKey, addressBytes...), statistic)
}

func IncreaseRewardDelegation(ctx contract.Context, validator *types.Address, delegator *types.Address, increase diadem.BigUInt) error {
	// check if rewards delegation already exists
	delegation, err := GetDelegation(ctx, REWARD_DELEGATION_INDEX, *validator, *delegator)
	if err == contract.ErrNotFound {
		delegation = &Delegation{
			Validator:    validator,
			Delegator:    delegator,
			Amount:       diadem.BigZeroPB(),
			UpdateAmount: diadem.BigZeroPB(),
			// rewards delegations are automatically unlocked
			LocktimeTier: 0,
			LockTime:     0,
			State:        BONDED,
			Index:        REWARD_DELEGATION_INDEX,
		}
	} else if err != nil {
		return err
	}

	// increase delegation amount by new reward amount
	updatedAmount := common.BigZero()
	updatedAmount.Add(&delegation.Amount.Value, &increase)
	delegation.Amount = &types.BigUInt{Value: *updatedAmount}

	return SetDelegation(ctx, delegation)
}

type CandidateList []*Candidate

func (c CandidateList) Get(addr diadem.Address) *Candidate {
	for _, cand := range c {
		if cand.Address.Local.Compare(addr.Local) == 0 {
			return cand
		}
	}
	return nil
}

func GetCandidate(ctx contract.StaticContext, addr diadem.Address) *Candidate {
	c, err := loadCandidateList(ctx)
	if err != nil {
		return nil
	}

	for _, cand := range c {
		if cand.Address.Local.Compare(addr.Local) == 0 {
			return cand
		}
	}
	return nil
}

func GetCandidateByPubKey(ctx contract.StaticContext, pubkey []byte) *Candidate {
	c, err := loadCandidateList(ctx)
	if err != nil {
		return nil
	}

	for _, cand := range c {
		if bytes.Compare(cand.PubKey, pubkey) == 0 {
			return cand
		}
	}
	return nil
}

func (c *CandidateList) Set(cand *Candidate) {
	found := false
	candAddr := diadem.UnmarshalAddressPB(cand.Address)
	for _, candidate := range *c {
		addr := diadem.UnmarshalAddressPB(candidate.Address)
		if candAddr.Local.Compare(addr.Local) == 0 {
			candidate = cand
			found = true
			break
		}
	}
	if !found {
		*c = append(*c, cand)
	}
}

func (c *CandidateList) Delete(addr diadem.Address) {
	newcl := *c
	for i, cand := range newcl {
		candAddr := diadem.UnmarshalAddressPB(cand.Address)
		if candAddr.Local.Compare(addr.Local) == 0 {
			copy(newcl[i:], newcl[i+1:])
			newcl = newcl[:len(newcl)-1]
			break
		}
	}
	*c = newcl
}

// Updates Unregistration and ChangeFee States
func updateCandidateList(ctx contract.Context) error {
	candidates, err := loadCandidateList(ctx)
	if err != nil {
		return err
	}

	// Update each candidate's fee
	var deleteList []diadem.Address
	for _, c := range candidates {
		if c.State == ABOUT_TO_CHANGE_FEE {
			c.State = CHANGING_FEE
		} else if c.State == CHANGING_FEE {
			c.Fee = c.NewFee
			c.State = REGISTERED
		} else if c.State == UNREGISTERING {
			deleteList = append(deleteList, diadem.UnmarshalAddressPB(c.Address))
		}
	}

	// Remove unregistering candidates from candidates array
	for _, candidateAddress := range deleteList {
		candidates.Delete(candidateAddress)
	}

	if err = saveCandidateList(ctx, candidates); err != nil {
		return err
	}

	return nil
}

type byAddress CandidateList

func (s byAddress) Len() int {
	return len(s)
}

func (s byAddress) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}

func (s byAddress) Less(i, j int) bool {
	vaddr1 := diadem.UnmarshalAddressPB(s[i].Address)
	vaddr2 := diadem.UnmarshalAddressPB(s[j].Address)
	diff := vaddr1.Local.Compare(vaddr2.Local)
	return diff < 0
}

func saveCandidateList(ctx contract.Context, cl CandidateList) error {
	sorted := sortCandidates(cl)
	return ctx.Set(candidatesKey, &dtypes.CandidateList{Candidates: sorted})
}

func loadCandidateList(ctx contract.StaticContext) (CandidateList, error) {
	var pbcl dtypes.CandidateList
	err := ctx.Get(candidatesKey, &pbcl)
	if err == contract.ErrNotFound {
		return CandidateList{}, nil
	}
	if err != nil {
		return nil, err
	}
	return pbcl.Candidates, nil
}

func GetReferrer(ctx contract.StaticContext, name string) *types.Address {
	var address types.Address
	err := ctx.Get(append(referrersKey, name...), &address)
	if err != nil {
		return nil
	}
	return &address
}

func SetReferrer(ctx contract.Context, name string, address *types.Address) error {
	return ctx.Set(append(referrersKey, name...), address)
}

func saveState(ctx contract.Context, state *State) error {
	state.Validators = sortValidators(state.Validators)
	return ctx.Set(stateKey, state)
}

func loadState(ctx contract.StaticContext) (*State, error) {
	var state State
	err := ctx.Get(stateKey, &state)
	if err != nil {
		return nil, err
	}

	return &state, nil
}

type DelegationResult struct {
	ValidatorAddress diadem.Address
	DelegationTotal  diadem.BigUInt
}

type byDelegationTotal []*DelegationResult

func (s byDelegationTotal) Len() int {
	return len(s)
}

func (s byDelegationTotal) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}

func (s byDelegationTotal) Less(i, j int) bool {
	diff := int64(s[i].DelegationTotal.Cmp(&s[j].DelegationTotal))
	if diff == 0 {
		// make sure output is deterministic if power is equal
		diff = int64(s[i].ValidatorAddress.Compare(s[j].ValidatorAddress))
	}

	return diff > 0
}

// Returns the elements of `former` which are not included in `current`
// `former` and `current` are always assumed to be sorted since validator lists
// are only stored as sorted arrays in `Params.State`
func MissingValidators(former, current []*Validator) []*Validator {
	var validators []*Validator

	var i, j int
	for j < len(former) {
		if i >= len(current) {
			validators = append(validators, former[j:]...)
			break
		}

		switch bytes.Compare(former[j].PubKey, current[i].PubKey) {
		case -1:
			validators = append(validators, former[j])
			j++
		case 0:
			i++
			j++
		case 1:
			i++
		}
	}

	return validators
}

// BATCH REQUESTS

func loadRequestBatchTally(ctx contract.StaticContext) (*RequestBatchTally, error) {
	tally := RequestBatchTally{}

	if err := ctx.Get(requestBatchTallyKey, &tally); err != nil {
		if err == contract.ErrNotFound {
			return &tally, nil
		}
		return nil, err
	}

	return &tally, nil
}

func saveRequestBatchTally(ctx contract.Context, tally *RequestBatchTally) error {
	return ctx.Set(requestBatchTallyKey, tally)
}
