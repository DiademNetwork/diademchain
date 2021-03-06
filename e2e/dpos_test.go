package main

import (
	"testing"
	"time"

	"github.com/diademnetwork/diademchain/e2e/common"
)

func TestContractDPOS(t *testing.T) {
	tests := []struct {
		name       string
		testFile   string
		validators int // TODO this is more like # of nodes than validators
		// # of validators is set in genesis params...
		accounts int
		genFile  string
		yamlFile string
	}{
		// {"dpos-downtime", "dpos-downtime.toml", 4, 10, "dpos.genesis.json", "dpos-test-diadem.yaml"},
		{"dpos-v3", "dposv3-delegation.toml", 4, 10, "dposv3.genesis.json", "dposv3-test-diadem.yaml"},
		{"dpos-delegation", "dpos-delegation.toml", 4, 10, "dpos-delegation.genesis.json", "dpos-test-diadem.yaml"},
		{"dpos-2", "dpos-2-validators.toml", 2, 10, "dpos.genesis.json", "dpos-test-diadem.yaml"},
		{"dpos-2-r2", "dpos-2-validators.toml", 2, 10, "dpos.genesis.json", "dpos-test-diadem.yaml"},
		{"dpos-4", "dpos-4-validators.toml", 4, 10, "dpos.genesis.json", "dpos-test-diadem.yaml"},
		{"dpos-4-r2", "dpos-4-validators.toml", 4, 10, "dpos.genesis.json", "dpos-test-diadem.yaml"},
		// {"dpos-8", "dpos-8-validators.toml", 8, 10, "dpos.genesis.json", "dpos-test-diadem.yaml"},
		{"dpos-elect-time", "dpos-elect-time-2-validators.toml", 2, 10, "dpos-elect-time.genesis.json", "dpos-test-diadem.yaml"},
	}

	common.DiademPath = "../diadem"
	common.ContractDir = "../contracts"

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			config, err := common.NewConfig(test.name, test.testFile, test.genFile, test.yamlFile, test.validators, test.accounts, 0)
			if err != nil {
				t.Fatal(err)
			}

			if err := common.DoRun(*config); err != nil {
				t.Fatal(err)
			}

			// pause before running the next test
			time.Sleep(500 * time.Millisecond)
		})
	}
}
