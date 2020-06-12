package karma

import (
	"encoding/binary"

	"github.com/diademnetwork/go-diadem/plugin/contractpb"
	"github.com/diademnetwork/diademchain"
	"github.com/diademnetwork/diademchain/builtin/plugins/karma"
)

var (
	// TODO: eliminate
	lastKarmaUpkeepKey = []byte("last:upkeep:karma")
)

func NewKarmaHandler(karmaContractCtx contractpb.Context) diademchain.KarmaHandler {
	return &karmaHandler{
		karmaContractCtx: karmaContractCtx,
	}
}

type karmaHandler struct {
	karmaContractCtx contractpb.Context
}

func (kh *karmaHandler) Upkeep() error {
	return karma.Upkeep(kh.karmaContractCtx)
}

func UintToBytesBigEndian(height uint64) []byte {
	heightB := make([]byte, 8)
	binary.BigEndian.PutUint64(heightB, height)
	return heightB
}
