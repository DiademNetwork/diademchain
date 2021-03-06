package db

import (
	"fmt"

	"github.com/diademnetwork/diademchain/db/metrics"
	"github.com/diademnetwork/diademchain/log"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/syndtr/goleveldb/leveldb/util"
	dbm "github.com/tendermint/tendermint/libs/db"
)

type GoLevelDB struct {
	*dbm.GoLevelDB
}

var _ DBWrapper = &GoLevelDB{}

func (g *GoLevelDB) Compact() error {
	return g.DB().CompactRange(util.Range{})
}

func (g *GoLevelDB) GetSnapshot() Snapshot {
	snap, err := g.DB().GetSnapshot()
	if err != nil {
		panic(err)
	}
	return &GoLevelDBSnapshot{
		Snapshot: snap,
	}
}

func LoadGoLevelDB(name, dir string, cacheSizeMeg int, collectMetrics bool) (*GoLevelDB, error) {
	o := &opt.Options{
		BlockCacheCapacity: cacheSizeMeg * opt.MiB,
	}
	db, err := dbm.NewGoLevelDBWithOpts(name, dir, o)
	if err != nil {
		return nil, err
	}

	if collectMetrics {
		err := prometheus.Register(metrics.NewStatsCollector(fmt.Sprintf("goleveldb_%s", name), log.Default, db))
		if err != nil {
			db.Close()
			return nil, errors.Wrap(err, "failed to register GoLevelDB stats collector")
		}
	}
	return &GoLevelDB{GoLevelDB: db}, nil
}
