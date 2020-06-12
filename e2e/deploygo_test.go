package main

import (
	"testing"
	"time"

	"github.com/diademnetwork/diademchain/e2e/common"
)

func TestDeployGoE2E(t *testing.T) {
	tests := []struct {
		name       string
		testFile   string
		validators int
		accounts   int
		genFile    string
		yamlFile   string
	}{
		{"deployGo", "deploygo-1-test.toml", 4, 2, "empty-genesis.json", ""},
		{"whitelist", "deploygo-2-test.toml", 1, 3, "empty-genesis.json", "deploygo-2-diadem.yaml"},
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
