// +build evm

package main

import (
	"encoding/json"
	"log"
	"net/http"
	"path/filepath"

	"github.com/diademnetwork/diademchain/gateway"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/spf13/viper"
)

type DiademConfig struct {
	ChainID                 string
	RPCProxyPort            int32
	DiademCoinTransferGateway *gateway.TransferGatewayConfig
}

func main() {
	cfg, err := parseConfig(nil)
	if err != nil {
		panic(err)
	}

	orc, err := gateway.CreateDiademCoinOracle(cfg.DiademCoinTransferGateway, cfg.ChainID)
	if err != nil {
		panic(err)
	}

	go orc.RunWithRecovery()

	http.HandleFunc("/status", func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(orc.Status())
	})

	http.Handle("/metrics", promhttp.Handler())

	log.Fatal(http.ListenAndServe(cfg.DiademCoinTransferGateway.OracleQueryAddress, nil))
}

// Loads diadem.yml or equivalent from one of the usual location, or if overrideCfgDirs is provided
// from one of those config directories.
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
		ChainID:                 "default",
		RPCProxyPort:            46658,
		DiademCoinTransferGateway: gateway.DefaultDiademCoinTGConfig(46658),
	}
	err := v.Unmarshal(conf)
	if err != nil {
		return nil, err
	}
	return conf, err
}
