// +build !evm

package config

import "github.com/diademnetwork/diademchain/builtin/plugins/dposv2/oracle"

func LoadSerializableConfig(chainID string, serializableConfig *OracleSerializableConfig) (*oracle.Config, error) {
	return &oracle.Config{
		Enabled: false,
	}, nil
}
