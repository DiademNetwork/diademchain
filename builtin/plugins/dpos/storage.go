package dpos

import (
	"bytes"
	"sort"

	diadem "github.com/diademnetwork/go-diadem"
	types "github.com/diademnetwork/go-diadem/builtin/types/dpos"
	contract "github.com/diademnetwork/go-diadem/plugin/contractpb"
	"github.com/diademnetwork/go-diadem/util"
)

var (
	economyKey    = []byte("economy")
	stateKey      = []byte("state")
	candidatesKey = []byte("candidates")
)

func addrKey(addr diadem.Address) string {
	return string(addr.Bytes())
}

func sortWitnesses(witnesses []*Witness) []*Witness {
	sort.Sort(byPubkey(witnesses))
	return witnesses
}

func sortCandidates(cands []*Candidate) []*Candidate {
	sort.Sort(byAddress(cands))
	return cands
}

func sortVotes(votes []*types.Vote) []*types.Vote {
	sort.Sort(byAddressAndAmount(votes))
	return votes
}

type byPubkey []*Witness

func (s byPubkey) Len() int {
	return len(s)
}

func (s byPubkey) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}

func (s byPubkey) Less(i, j int) bool {
	return bytes.Compare(s[i].PubKey, s[j].PubKey) < 0
}

type VoteList []*types.Vote

func (vl VoteList) Get(addr diadem.Address) *types.Vote {
	for _, v := range vl {
		addrV := diadem.UnmarshalAddressPB(v.VoterAddress)
		if addr.Local.Compare(addrV.Local) == 0 {
			return v
		}
	}
	return nil
}

func (vl *VoteList) Set(vote *types.Vote) {
	addr := diadem.UnmarshalAddressPB(vote.VoterAddress)
	found := false
	for _, v := range *vl {
		addrV := diadem.UnmarshalAddressPB(v.VoterAddress)
		if addr.Local.Compare(addrV.Local) == 0 {
			v = vote
			found = true
			break
		}
	}
	if !found {
		*vl = append(*vl, vote)
	}
}

type byAddressAndAmount []*types.Vote

func (s byAddressAndAmount) Len() int {
	return len(s)
}

func (s byAddressAndAmount) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}

func (s byAddressAndAmount) Less(i, j int) bool {
	vaddr1 := diadem.UnmarshalAddressPB(s[i].VoterAddress)
	vaddr2 := diadem.UnmarshalAddressPB(s[j].VoterAddress)
	diff := vaddr1.Local.Compare(vaddr2.Local)
	if diff == 0 {
		caddr1 := diadem.UnmarshalAddressPB(s[i].CandidateAddress)
		caddr2 := diadem.UnmarshalAddressPB(s[j].CandidateAddress)
		diff = caddr1.Local.Compare(caddr2.Local)

		if diff == 0 {
			return s[i].Amount < s[j].Amount
		}
	}

	return diff < 0
}

type CandidateList []*types.Candidate

func (c CandidateList) Get(addr diadem.Address) *Candidate {
	for _, cand := range c {
		if cand.Address.Local.Compare(addr.Local) == 0 {
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
	var newcl CandidateList
	for _, cand := range *c {
		candAddr := diadem.UnmarshalAddressPB(cand.Address)
		addr := diadem.UnmarshalAddressPB(cand.Address)
		if candAddr.Local.Compare(addr.Local) != 0 {
			newcl = append(newcl, cand)
		}
	}
	*c = newcl
}

type byAddress []*types.Candidate

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
	return ctx.Set(candidatesKey, &types.CandidateList{Candidates: sorted})
}

func loadCandidateList(ctx contract.StaticContext) (CandidateList, error) {
	var pbcl types.CandidateList
	err := ctx.Get(candidatesKey, &pbcl)
	if err == contract.ErrNotFound {
		return CandidateList{}, nil
	}
	if err != nil {
		return nil, err
	}
	return pbcl.Candidates, nil
}

func voterKey(addr diadem.Address) []byte {
	return util.PrefixKey([]byte("voter"), addr.Bytes())
}

func saveVoter(ctx contract.Context, v *types.Voter) error {
	addr := diadem.UnmarshalAddressPB(v.Address)
	return ctx.Set(voterKey(addr), v)
}

func loadVoter(ctx contract.Context, addr diadem.Address, defaultBalance uint64) (*types.Voter, error) {
	v := types.Voter{
		Address: addr.MarshalPB(),
		Balance: defaultBalance,
	}
	err := ctx.Get(voterKey(addr), &v)
	if err != nil && err != contract.ErrNotFound {
		return nil, err
	}

	return &v, nil
}

func voteSetKey(addr diadem.Address) []byte {
	return util.PrefixKey([]byte("votes"), addr.Bytes())
}

func saveVoteSet(ctx contract.Context, candAddr diadem.Address, vs VoteList) error {
	sorted := sortVotes(vs)
	return ctx.Set(voteSetKey(candAddr), &types.VoteList{Votes: sorted})
}

func loadVoteSet(ctx contract.StaticContext, candAddr diadem.Address) (VoteList, error) {
	var pbvs types.VoteList
	err := ctx.Get(voteSetKey(candAddr), &pbvs)
	if err == contract.ErrNotFound {
		return VoteList{}, nil
	}
	if err != nil {
		return nil, err
	}

	return pbvs.Votes, nil
}

func saveState(ctx contract.Context, state *types.State) error {
	return ctx.Set(stateKey, state)
}

func loadState(ctx contract.StaticContext) (*types.State, error) {
	var state types.State
	err := ctx.Get(stateKey, &state)
	if err != nil {
		return nil, err
	}

	return &state, nil
}
