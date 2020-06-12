package bdiadem

import (
	"github.com/diademnetwork/go-diadem/plugin/types"
	"github.com/syndtr/goleveldb/leveldb/filter"
	"github.com/syndtr/goleveldb/leveldb/util"
)

var (
	BitsPerKey = 8
)

func NewBdiademFilter() filter.Filter {
	return filter.NewBdiademFilter(BitsPerKey)
}

func GenBdiademFilter(msgs []*types.EventData) []byte {
	if len(msgs) == 0 {
		return []byte{}
	} else {
		bdiademFilter := filter.NewBdiademFilter(BitsPerKey)
		generator := bdiademFilter.NewGenerator()

		for _, msg := range msgs {
			for _, topic := range msg.Topics {
				generator.Add([]byte(topic))
			}
			generator.Add(msg.Address.Local)
		}
		buff := &util.Buffer{}
		generator.Generate(buff)
		return buff.Bytes()
	}
}
