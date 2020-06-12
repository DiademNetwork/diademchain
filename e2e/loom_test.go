package main

import (
	"fmt"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/diademnetwork/diademchain/e2e/common"
)

func TestE2eEvm(t *testing.T) {
	tests := []struct {
		name        string
		testFile    string
		validators  int
		accounts    int
		ethAccounts int
		genFile     string
		yamlFile    string
	}{
		{"evm", "diadem-1-test.toml", 4, 10, 0, "empty-genesis.json", "diadem.yaml"},
		{"deployEnable", "diadem-2-test.toml", 4, 10, 0, "empty-genesis.json", "diadem-2-diadem.yaml"},
		{"ethSignature-type1", "diadem-3-test.toml", 1, 1, 1, "diadem-3-genesis.json", "diadem-3-diadem.yaml"},
		{"ethSignature-type2", "diadem-4-test.toml", 1, 2, 2, "diadem-4-genesis.json", "diadem-4-diadem.yaml"},
		{"migration-tx", "diadem-5-test.toml", 3, 3, 3, "diadem-5-genesis.json", "diadem-5-diadem.yaml"},
		{"evm-state-migration", "diadem-6-test.toml", 4, 4, 4, "diadem-6-genesis.json", "diadem-6-diadem.yaml"},
	}
	common.DiademPath = "../diadem"
	common.ContractDir = "../contracts"

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			config, err := common.NewConfig(test.name, test.testFile, test.genFile, test.yamlFile, test.validators, test.accounts, test.ethAccounts)
			if err != nil {
				t.Fatal(err)
			}

			binary, err := exec.LookPath("go")
			if err != nil {
				t.Fatal(err)
			}

			exampleCmd := exec.Cmd{
				Dir:  config.BaseDir,
				Path: binary,
				Args: []string{binary, "build", "-tags", "evm", "-o", "example-cli", "github.com/diademnetwork/go-diadem/examples/cli"},
			}

			if err := exampleCmd.Run(); err != nil {
				t.Fatal(fmt.Errorf("fail to execute command: %s\n%v", strings.Join(exampleCmd.Args, " "), err))
			}

			if err := common.DoRun(*config); err != nil {
				t.Fatal(err)
			}

			// pause before running the next test
			time.Sleep(500 * time.Millisecond)
		})
	}
}
