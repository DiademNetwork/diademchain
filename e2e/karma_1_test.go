package main

import (
	"fmt"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/diademnetwork/diademchain/e2e/common"
)

func TestE2eKarma(t *testing.T) {
	tests := []struct {
		name       string
		testFile   string
		validators int
		accounts   int
		genFile    string
		yamlFile   string
	}{
		{"karma", "karma-1-test.toml", 1, 3, "karma-1-test.json", "karma-1-diadem.yaml"},
		{"coin", "karma-2-test.toml", 1, 4, "karma-2-test.json", "karma-2-diadem.yaml"},
		{"upkeep", "karma-3-test.toml", 1, 4, "karma-3-test.json", "karma-3-diadem.yaml"},
		{"config", "karma-4-test.toml", 1, 2, "karma-4-test.json", "karma-3-diadem.yaml"},
	}
	common.DiademPath = "../diadem"
	common.ContractDir = "../contracts"

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			config, err := common.NewConfig(test.name, test.testFile, test.genFile, test.yamlFile, test.validators, test.accounts, 0)
			if err != nil {
				t.Fatal(err)
			}

			binary, err := exec.LookPath("go")
			if err != nil {
				t.Fatal(err)
			}

			if test.name == "coin" {
				cmdExampleCli := exec.Cmd{
					Dir:  config.BaseDir,
					Path: binary,
					Args: []string{binary, "build", "-tags", "evm", "-o", "example-cli", "github.com/diademnetwork/go-diadem/examples/cli"},
				}
				if err := cmdExampleCli.Run(); err != nil {
					t.Fatal(fmt.Errorf("fail to execute command: %s\n%v", strings.Join(cmdExampleCli.Args, " "), err))
				}
			}

			if err := common.DoRun(*config); err != nil {
				t.Fatal(err)
			}

			// pause before running the next test
			time.Sleep(500 * time.Millisecond)
		})
	}
}
