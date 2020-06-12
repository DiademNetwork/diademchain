package common

import (
	"path/filepath"

	"github.com/diademnetwork/diademchain/config"
	"github.com/spf13/viper"
)

// Loads diadem.yml from ./ or ./config
func ParseConfig() (*config.Config, error) {
	v := viper.New()
	v.AutomaticEnv()
	v.SetEnvPrefix("DIADEM")

	v.SetConfigName("diadem")                       // name of config file (without extension)
	v.AddConfigPath(".")                          // search root directory
	v.AddConfigPath(filepath.Join(".", "config")) // search root directory /config
	v.AddConfigPath("./../../../")

	v.ReadInConfig()
	conf := config.DefaultConfig()
	err := v.Unmarshal(conf)
	if err != nil {
		return nil, err
	}
	return conf, err
}
