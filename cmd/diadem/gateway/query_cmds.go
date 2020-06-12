// +build evm

package gateway

import (
	"encoding/json"
	"fmt"
	"log"
	"math/big"
	"strings"

	"github.com/diademnetwork/diademchain/builtin/plugins/ethcoin"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/diademnetwork/go-diadem"
	ctypes "github.com/diademnetwork/go-diadem/builtin/types/coin"
	tgtypes "github.com/diademnetwork/go-diadem/builtin/types/transfer_gateway"
	"github.com/diademnetwork/go-diadem/client"
	"github.com/diademnetwork/go-diadem/client/erc20"
	"github.com/diademnetwork/go-diadem/client/gateway"
	"github.com/diademnetwork/diademchain/builtin/plugins/coin"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

type accountInfo struct {
	DAppChainAddress string
	EthereumAddress  string
	DIADEM             string
	ETH              string
}

const queryAccountCmdExample = `
# Get info about a DAppChain account
./diadem gateway account 0x2a6b071aD396cEFdd16c731454af0d8c95ECD4B2

# Get info about an Ethereum account
./diadem gateway account eth:0x5d1ddf5223a412d24901c32d14ef56cb706c0f64
`

func newQueryAccountCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "account <account-addr>",
		Short:   "Displays information about a DAppChain or Ethereum account known to the Transfer Gateway.",
		Example: queryAccountCmdExample,
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var addr diadem.Address
			var err error
			if strings.HasPrefix(args[0], "eth:") {
				addr, err = diadem.ParseAddress(args[0])
			} else {
				if strings.HasPrefix(args[0], gatewayCmdFlags.ChainID+":") {
					addr, err = diadem.ParseAddress(args[0])
				} else {
					addr, err = hexToDiademAddress(args[0])
				}
			}
			if err != nil {
				return errors.Wrap(err, "invalid account address")
			}

			rpcClient := getDAppChainClient()
			mapperAddr, err := rpcClient.Resolve("addressmapper")
			if err != nil {
				return errors.Wrap(err, "failed to resolve DAppChain Address Mapper address")
			}
			mapper := client.NewContract(rpcClient, mapperAddr.Local)
			mappedAccount, err := getMappedAccount(mapper, addr)
			if err != nil {
				fmt.Printf("No account information found for %v", addr)
			}

			var localAddr, foreignAddr diadem.Address
			if addr.ChainID == "eth" {
				localAddr = mappedAccount
				foreignAddr = addr
			} else {
				localAddr = addr
				foreignAddr = mappedAccount
			}

			info := &accountInfo{
				DAppChainAddress: localAddr.Local.String(),
				EthereumAddress:  common.BytesToAddress(foreignAddr.Local).Hex(),
			}

			coinAddr, err := rpcClient.Resolve("coin")
			if err == nil {
				coinContract := client.NewContract(rpcClient, coinAddr.Local)
				req := &ctypes.BalanceOfRequest{
					Owner: localAddr.MarshalPB(),
				}
				var resp ctypes.BalanceOfResponse
				_, err = coinContract.StaticCall("BalanceOf", req, localAddr, &resp)
				if err != nil {
					return errors.Wrap(err, "failed to call coin.BalanceOf")
				}
				balance := new(big.Int)
				if resp.Balance != nil {
					balance = resp.Balance.Value.Int
				}
				info.DIADEM = fmt.Sprintf(
					"%s (%s)",
					formatTokenAmount(balance), balance.String(),
				)
			}

			ethCoinAddr, err := rpcClient.Resolve("ethcoin")
			if err == nil {
				coinContract := client.NewContract(rpcClient, ethCoinAddr.Local)
				req := &ctypes.BalanceOfRequest{
					Owner: localAddr.MarshalPB(),
				}
				var resp ctypes.BalanceOfResponse
				_, err = coinContract.StaticCall("BalanceOf", req, localAddr, &resp)
				if err != nil {
					return errors.Wrap(err, "failed to call ethcoin.BalanceOf")
				}
				balance := new(big.Int)
				if resp.Balance != nil {
					balance = resp.Balance.Value.Int
				}
				info.ETH = fmt.Sprintf(
					"%s (%s)",
					formatTokenAmount(balance), balance.String(),
				)
			}

			output, err := json.MarshalIndent(info, "", "  ")
			if err != nil {
				return err
			}
			fmt.Println(string(output))
			return nil
		},
	}
	return cmd
}

//Converts the given amount to a human readable string by stripping off 18 decimal places.
func formatTokenAmount(amount *big.Int) string {
	divisor := big.NewInt(10)
	divisor.Exp(divisor, big.NewInt(18), nil)
	return new(big.Int).Div(amount, divisor).String()
}

const queryUnclaimedTokensCmdExample = `
# Show unclaimed DIADEM in the DAppChain Gateway deposited by an Ethereum account
./diadem gateway unclaimed-tokens diademcoin-gateway 0x2a6b071aD396cEFdd16c731454af0d8c95ECD4B2

# Show unclaimed tokens in the DAppChain Gateway deposited by an Ethereum account
./diadem gateway unclaimed-tokens eth:0x5d1ddf5223a412d24901c32d14ef56cb706c0f64
`

func newQueryUnclaimedTokensCommand() *cobra.Command {
	var gatewayName string
	cmd := &cobra.Command{
		Use:     "unclaimed-tokens <account-addr> [gateway-name]",
		Short:   "Shows unclaimed tokens in the Transfer Gateway deposited by an Ethereum account",
		Example: queryUnclaimedTokensCmdExample,
		Args:    cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var addr diadem.Address
			var err error
			if strings.HasPrefix(args[0], "eth:") {
				addr, err = diadem.ParseAddress(args[0])
			} else {
				if strings.HasPrefix(args[0], gatewayCmdFlags.ChainID+":") {
					return errors.Wrap(err, "account address is not an Ethereum address")
				} else {
					local, err := diadem.LocalAddressFromHexString(args[0])
					if err != nil {
						return errors.Wrap(err, "failed to parse account address")
					}
					addr = diadem.Address{ChainID: "eth", Local: local}
				}
			}
			if err != nil {
				return errors.Wrap(err, "invalid account address")
			}

			gatewayName := GatewayName
			if len(args) > 1 {
				if strings.EqualFold(args[1], DiademGatewayName) {
					gatewayName = DiademGatewayName
				} else if !strings.EqualFold(args[1], GatewayName) {
					return errors.New("Invalid gateway name")
				}
			}

			rpcClient := getDAppChainClient()
			gatewayAddr, err := rpcClient.Resolve(gatewayName)
			if err != nil {
				return errors.Wrap(err, "failed to resolve DAppChain Gateway address")
			}
			gateway := client.NewContract(rpcClient, gatewayAddr.Local)

			req := &tgtypes.TransferGatewayGetUnclaimedTokensRequest{
				Owner: addr.MarshalPB(),
			}
			resp := &tgtypes.TransferGatewayGetUnclaimedTokensResponse{}
			_, err = gateway.StaticCall("GetUnclaimedTokens", req, addr, resp)
			if err != nil {
				return errors.Wrap(err, "failed to call GetUnclaimedTokens on Gateway contract")
			}
			output, err := json.MarshalIndent(resp.UnclaimedTokens, "", "  ")
			if err != nil {
				return err
			}
			fmt.Println(string(output))
			return nil
		},
	}
	cmd.Flags().StringVarP(
		&gatewayName, "gateway", "g", GatewayName,
		"Which Gateway contract to query, gateway or diademcoin-gateway",
	)
	return cmd
}

type Supply struct {
	Ethereum Eth  `json:"ethereum"`
	DiademCoin Diadem `json:"diademcoin"`
}

type Eth struct {
	DappchainTotalSupply       string `json:"dappchain_total_supply"`
	DappchainCirculatingSupply string `json:"dappchain_circulating_supply"`
	DappchainGatewayTotal      string `json:"dappchain_gateway_total"`
	EthereumGatewayTotal       string `json:"ethereum_total_supply"`
	DappchainGatewayUnclaimed  string `json:"dappchain_gateway_unclaimed"`
}

type Diadem struct {
	DappchainTotalSupply      string `json:"dappchain_total_supply"`
	DappchainGatewayTotal     string `json:"dappchain_gateway_total"`
	EthereumGatewayTotal      string `json:"ethereum_gateway_total"`
	DappchainGatewayUnclaimed string `json:"dappchain_gateway_unclaimed"`
}

const queryGatewaySupplyCmdExample = `
# Show holdings of DAppChain & Ethereum Gateways
./diadem gateway supply \
   --eth-uri https://mainnet.infura.io/v3/a5a5151fecba45229aa77f0725c10241 \
   --eth-gateway-addr 0x223CA78df868367D214b444d561B9123c018963A \
   --diadem-eth-addr 0xa4e8c3ec456107ea67d3075bf9e3df3a75823db0 \
   --diadem-eth-gateway-addr 0x8f8E8b3C4De76A31971Fe6a87297D8f703bE8570 \
   --chain default \
   --uri http://plasma.dappchains.com:80
`

func newQueryGatewaySupplyCommand() *cobra.Command {
	var ethURI, gatewayAddressEth, diademCoinAddressEth, diademGatewayAddressEth string
	var raw bool
	cmd := &cobra.Command{
		Use:     "supply",
		Short:   "Displays holdings of DAppChain & Ethereum Gateways",
		Example: queryGatewaySupplyCmdExample,
		Args:    cobra.MinimumNArgs(0),
		RunE: func(cmd *cobra.Command, args []string) error {
			var supply Supply
			rpcClient := getDAppChainClient()

			ethClient, err := ethclient.Dial(ethURI)
			if err != nil {
				return err
			}
			gatewayClient, err := gateway.ConnectToMainnetGateway(ethClient, gatewayAddressEth)
			if err != nil {
				return err
			}
			balance, err := gatewayClient.ETHBalance()
			if err != nil {
				return err
			}
			if raw {
				supply.Ethereum.EthereumGatewayTotal = balance.String()
			} else {
				supply.Ethereum.EthereumGatewayTotal = fmt.Sprintf(
					"%s (%s)",
					formatTokenAmount(balance), balance.String(),
				)
			}

			ethCoinAddr, err := rpcClient.Resolve("ethcoin")
			if err != nil {
				return errors.Wrap(err, "failed to resolve ethCoin address")
			}
			ethcoinContract := client.NewContract(rpcClient, ethCoinAddr.Local)
			tsreq1 := ethcoin.TotalSupplyRequest{}
			var tsresp1 ethcoin.TotalSupplyResponse
			_, err = ethcoinContract.StaticCall("TotalSupply", &tsreq1, ethCoinAddr, &tsresp1)
			if err != nil {
				return err
			}
			ethCoinSupply := tsresp1.TotalSupply.Value.Int
			if raw {
				supply.Ethereum.DappchainTotalSupply = ethCoinSupply.String()
			} else {
				supply.Ethereum.DappchainTotalSupply = fmt.Sprintf(
					"%s (%s)", formatTokenAmount(ethCoinSupply), ethCoinSupply.String(),
				)
			}

			erc20client, err := erc20.ConnectToMainnetERC20(ethClient, diademCoinAddressEth)
			if err != nil {
				return err
			}
			diademGatewayEthereumAddress := common.HexToAddress(diademGatewayAddressEth)
			diademCoinsEthDiademGateway, err := erc20client.BalanceOf(diademGatewayEthereumAddress)
			if err != nil {
				return err
			}
			if raw {
				supply.DiademCoin.EthereumGatewayTotal = diademCoinsEthDiademGateway.String()
			} else {
				supply.DiademCoin.EthereumGatewayTotal = fmt.Sprintf(
					"%s (%s)",
					formatTokenAmount(diademCoinsEthDiademGateway), diademCoinsEthDiademGateway.String(),
				)
			}

			gatewayAddr1, err := rpcClient.Resolve("gateway")
			if err != nil {
				return errors.Wrap(err, "failed to resolve Gateway address")
			}

			gBalanceRequest := &ctypes.BalanceOfRequest{
				Owner: gatewayAddr1.MarshalPB(),
			}

			var gBalanceResp ctypes.BalanceOfResponse
			_, err = ethcoinContract.StaticCall("BalanceOf", gBalanceRequest, gatewayAddr1, &gBalanceResp)
			if err != nil {
				return errors.Wrap(err, "failed to call ethcoin.BalanceOf")
			}

			gatewayEthCoinBalance := gBalanceResp.Balance.Value.Int
			if raw {
				supply.Ethereum.DappchainGatewayTotal = gatewayEthCoinBalance.String()
			} else {
				supply.Ethereum.DappchainGatewayTotal = fmt.Sprintf(
					"%s (%s)",
					formatTokenAmount(gatewayEthCoinBalance), gatewayEthCoinBalance.String(),
				)
			}
			ethCirculation := ethCoinSupply.Sub(ethCoinSupply, gatewayEthCoinBalance)
			if raw {
				supply.Ethereum.DappchainCirculatingSupply = ethCirculation.String()
			} else {
				supply.Ethereum.DappchainCirculatingSupply = fmt.Sprintf(
					"%s (%s)",
					formatTokenAmount(ethCirculation), ethCirculation.String(),
				)
			}

			gatewayAddr, err := rpcClient.Resolve("diademcoin-gateway")
			if err != nil {
				return errors.Wrap(err, "failed to resolve diademcoin Gateway address")
			}
			gatewayContract := client.NewContract(rpcClient, gatewayAddr.Local)
			ethLocalAddr, err := diadem.LocalAddressFromHexString(diademCoinAddressEth)
			if err != nil {
				return err
			}

			diademTokenEthereumAddr := diadem.Address{ChainID: "eth", Local: ethLocalAddr}
			req := &tgtypes.TransferGatewayGetUnclaimedContractTokensRequest{TokenAddress: diademTokenEthereumAddr.MarshalPB()}
			resp := &tgtypes.TransferGatewayGetUnclaimedContractTokensResponse{}
			_, err = gatewayContract.StaticCall("GetUnclaimedContractTokens", req, gatewayAddr, resp)
			if err != nil {
				log.Printf("Failed to retrieve unclaimed tokens. Error: %v", err)
			} else {
				unclaimedDIADEM := resp.UnclaimedAmount.Value
				supply.DiademCoin.DappchainGatewayUnclaimed = unclaimedDIADEM.String()
			}

			ethereumAddr := diadem.RootAddress("eth")
			req1 := &tgtypes.TransferGatewayGetUnclaimedContractTokensRequest{TokenAddress: ethereumAddr.MarshalPB()}
			resp1 := &tgtypes.TransferGatewayGetUnclaimedContractTokensResponse{}
			_, err = gatewayContract.StaticCall("GetUnclaimedContractTokens", req1, gatewayAddr, resp1)
			if err != nil {
				log.Printf("Failed to retrieve unclaimed tokens. Error: %v", err)
			} else {
				unclaimedETH := resp1.UnclaimedAmount.Value
				supply.Ethereum.DappchainGatewayUnclaimed = unclaimedETH.String()
			}

			coinAddr, err := rpcClient.Resolve("coin")
			if err != nil {
				return errors.Wrap(err, "failed to resolve coin address")
			}

			coinContract := client.NewContract(rpcClient, coinAddr.Local)
			tsreq := coin.TotalSupplyRequest{}
			var tsresp coin.TotalSupplyResponse
			_, err = coinContract.StaticCall("TotalSupply", &tsreq, coinAddr, &tsresp)
			if err != nil {
				return err
			}

			coinSupply := tsresp.TotalSupply.Value.Int
			if raw {
				supply.DiademCoin.DappchainTotalSupply = coinSupply.String()
			} else {
				supply.DiademCoin.DappchainTotalSupply = fmt.Sprintf(
					"%s (%s)", formatTokenAmount(coinSupply), coinSupply.String(),
				)
			}

			diademGatewayBalanceReq := &ctypes.BalanceOfRequest{
				Owner: gatewayAddr.MarshalPB(),
			}
			var diademGatewayBalanceResp ctypes.BalanceOfResponse
			_, err = coinContract.StaticCall("BalanceOf", diademGatewayBalanceReq, gatewayAddr, &diademGatewayBalanceResp)
			if err != nil {
				return errors.Wrap(err, "failed to call coin.BalanceOf")
			}

			diademGatewayCoinBalance := diademGatewayBalanceResp.Balance.Value.Int
			if raw {
				supply.DiademCoin.DappchainGatewayTotal = diademGatewayCoinBalance.String()
			} else {
				supply.DiademCoin.DappchainGatewayTotal = fmt.Sprintf(
					"%s (%s)",
					formatTokenAmount(diademGatewayCoinBalance), diademGatewayCoinBalance.String())
			}

			output, err := json.MarshalIndent(supply, "", "  ")
			if err != nil {
				return err
			}
			fmt.Println(string(output))
			return nil

		},
	}
	cmdFlags := cmd.Flags()
	cmdFlags.BoolVar(&raw, "raw", false, "raw format output")
	cmdFlags.StringVar(&ethURI, "eth-uri", "https://mainnet.infura.io/v3/a5a5151fecba45229aa77f0725c10241", "Ethereum URI")
	cmdFlags.StringVar(&gatewayAddressEth, "eth-gateway-addr", "0xE080079Ac12521D57573f39543e1725EA3E16DcC", "Ethereum Gateway Address")
	cmdFlags.StringVar(&diademCoinAddressEth, "diadem-eth-addr", "0xa4e8c3ec456107ea67d3075bf9e3df3a75823db0", "DIADEM Ethereum Contract Address")
	cmdFlags.StringVar(&diademGatewayAddressEth, "diadem-eth-gateway-addr", "0x8f8E8b3C4De76A31971Fe6a87297D8f703bE8570", "DIADEM Ethereum Gateway Address")
	return cmd
}
