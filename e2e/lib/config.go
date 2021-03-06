package lib

import (
	"bytes"
	"io/ioutil"
	"path"

	"github.com/BurntSushi/toml"
	"github.com/pkg/errors"

	"github.com/diademnetwork/diademchain/e2e/node"
)

type Config struct {
	Name        string
	BaseDir     string
	DiademPath    string
	ContractDir string
	Nodes       map[string]*node.Node
	Accounts    []*node.Account
	EthAccounts []*node.EthAccount
	TestFile    string
	LogAppDb    bool
	// helper to easy access by template
	AccountAddressList     []string
	AccountPrivKeyPathList []string
	AccountPubKeyList      []string

	EthAccountAddressList     []string
	EthAccountPrivKeyPathList []string
	EthAccountPubKeyList      []string

	NodeAddressList         []string
	NodePubKeyList          []string
	NodePrivKeyPathList     []string
	NodeRPCAddressList      []string
	NodeProxyAppAddressList []string
	// yaml file
	loglevel string
}

func WriteConfig(conf Config, filename string) error {
	buf := new(bytes.Buffer)
	if err := toml.NewEncoder(buf).Encode(conf); err != nil {
		return errors.Wrapf(err, "encoding runner config error")
	}

	configPath := path.Join(conf.BaseDir, filename)
	if err := ioutil.WriteFile(configPath, buf.Bytes(), 0644); err != nil {
		return err
	}
	return nil
}

func ReadConfig(filename string) (Config, error) {
	var conf Config
	if _, err := toml.DecodeFile(filename, &conf); err != nil {
		return conf, err
	}
	return conf, nil
}
