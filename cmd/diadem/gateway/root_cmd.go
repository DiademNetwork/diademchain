package gateway

import (
	diadem "github.com/diademnetwork/go-diadem"
	"github.com/spf13/cobra"
)

type gatewayFlags struct {
	ChainID        string
	URI            string
	HSMConfigPath  string
	PrivKeyPath    string
	EthPrivKeyPath string
	Algo           string
}

var gatewayCmdFlags gatewayFlags

func newRootCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "gateway <command>",
		Short: "Transfer Gateway Administration",
	}
	pflags := cmd.PersistentFlags()
	pflags.StringVarP(&gatewayCmdFlags.ChainID, "chain", "c", "default", "DAppChain ID")
	pflags.StringVarP(&gatewayCmdFlags.URI, "uri", "u", "http://localhost:46658", "DAppChain base URI")
	pflags.StringVarP(&gatewayCmdFlags.PrivKeyPath, "key", "k", "", "DAppChain Private Key file path")
	pflags.StringVarP(&gatewayCmdFlags.EthPrivKeyPath, "eth-key", "", "", "Ethereum Private Key file path")
	pflags.StringVarP(&gatewayCmdFlags.HSMConfigPath, "hsm", "", "", "HSM file path")
	pflags.StringVarP(&gatewayCmdFlags.Algo, "algo", "", "ed25519", "Signing algorithm")
	return cmd
}

//nolint:unused
func hexToDiademAddress(hexStr string) (diadem.Address, error) {
	addr, err := diadem.LocalAddressFromHexString(hexStr)
	if err != nil {
		return diadem.Address{}, err
	}
	return diadem.Address{
		ChainID: gatewayCmdFlags.ChainID,
		Local:   addr,
	}, nil
}
