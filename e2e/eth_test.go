package main

import (
	"testing"
	"time"

	"github.com/diademnetwork/diademchain/e2e/common"
)

func TestEthJSONRPC2(t *testing.T) {
	tests := []struct {
		name       string
		testFile   string
		validators int
		accounts   int
		genFile    string
		yamlFile   string
	}{
		{"blockNumber", "eth-1-test.toml", 1, 1, "empty-genesis.json", "eth-test-1-diadem.yaml"},
		{"ethPolls", "eth-2-test.toml", 1, 0, "empty-genesis.json", "eth-test-2-diadem.yaml"},
		{"getBlockByNumber", "eth-3-test.toml", 1, 1, "empty-genesis.json", "eth-test-2-diadem.yaml"},
		{"getBlockTransactionCountByNumber", "eth-4-test.toml", 1, 1, "empty-genesis.json", "eth-test-1-diadem.yaml"},
		{"getLogs", "eth-5-test.toml", 1, 4, "empty-genesis.json", "eth-test-2-diadem.yaml"},
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
