package main

import (
	"fmt"

	"github.com/diademnetwork/diademchain/store"
	dbm "github.com/tendermint/tendermint/libs/db"
)

var (
	prefixes = map[string]string{
		"Nonce":   "nonce",
		"Evm":     "vm",
		"Receipt": "receipt",
		"TxHash":  "txHash",
		"Bdiadem":   "bdiademFilter",
	}
)

func investigate(name, path string) error {
	db, err := dbm.NewGoLevelDB(name, path)
	if err != nil {
		return err
	}
	diademstore, err := store.NewIAVLStore(db, 0, 0)
	if err != nil {
		return err
	}
	fmt.Print("prefix\tnum keys\tsum sizes\n")
	for _, prefix := range prefixes {
		prefixRange := diademstore.Range([]byte(prefix))
		totalLength := 0
		for _, entry := range prefixRange {
			entry = entry
			totalLength += len(entry.Value)
		}
		fmt.Println(prefix, "\t", len(prefixRange), "\t", totalLength)
	}

	return nil
}
