// +build evm

package main

import (
	"encoding/json"
	"net/http"
	"path/filepath"

	"github.com/diademnetwork/diademchain/builtin/plugins/dposv2/oracle"
	"github.com/diademnetwork/diademchain/builtin/plugins/dposv2/oracle/config"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/spf13/viper"
)

const (
	DefaultStatusServiceAddress = "127.0.0.1:9996"
)

type DiademConfig struct {
	ChainID            string
	DPOSv2OracleConfig *config.OracleSerializableConfig
}

func main() {
	diademCfg, err := parseConfig(nil)
	if err != nil {
		panic(errors.Wrapf(err, "unable to parse diadem configuration"))
	}

	// Forcefully set this true as this is standlone oracle execution
	// This is required to load entire configuration
	diademCfg.DPOSv2OracleConfig.Enabled = true

	dposv2OracleConfig, err := config.LoadSerializableConfig(diademCfg.ChainID, diademCfg.DPOSv2OracleConfig)
	if err != nil {
		panic(errors.Wrapf(err, "unable to load dposv2 oracle configuration"))
	}

	oracle := oracle.NewOracle(dposv2OracleConfig)

	if err := oracle.Init(); err != nil {
		panic(errors.Wrapf(err, "unable to init oracle"))
	}

	go oracle.Run()

	http.HandleFunc("/status", func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(oracle.Status())
	})

	http.Handle("/metrics", promhttp.Handler())

	serviceStatusAddress := DefaultStatusServiceAddress
	if dposv2OracleConfig.StatusServiceAddress != "" {
		serviceStatusAddress = dposv2OracleConfig.StatusServiceAddress
	}

	err = http.ListenAndServe(serviceStatusAddress, nil)
	if err != nil {
		panic(errors.Wrapf(err, "unable to start status service for dposv2 oracle"))
	}
}

func parseConfig(overrideCfgDirs []string) (*DiademConfig, error) {
	v := viper.New()
	v.SetConfigName("diadem")

	if len(overrideCfgDirs) == 0 {
		// look for the diadem config file in all the places diadem itself does
		v.AddConfigPath(".")
		v.AddConfigPath(filepath.Join(".", "config"))
	} else {
		for _, dir := range overrideCfgDirs {
			v.AddConfigPath(dir)
		}
	}
	v.ReadInConfig()
	conf := &DiademConfig{
		ChainID:            "default",
		DPOSv2OracleConfig: config.DefaultConfig(),
	}
	err := v.Unmarshal(conf)
	if err != nil {
		return nil, err
	}
	return conf, err
}
