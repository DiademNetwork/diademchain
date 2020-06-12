package deployer_whitelist

import (
	"fmt"

	"github.com/diademnetwork/go-diadem"
	amtypes "github.com/diademnetwork/go-diadem/builtin/types/address_mapper"
	"github.com/diademnetwork/go-diadem/cli"
	"github.com/diademnetwork/go-diadem/client"
	"github.com/pkg/errors"
)

func getMappedAccount(mapper *client.Contract, account diadem.Address) (diadem.Address, error) {
	req := &amtypes.AddressMapperGetMappingRequest{
		From: account.MarshalPB(),
	}
	resp := &amtypes.AddressMapperGetMappingResponse{}
	_, err := mapper.StaticCall("GetMapping", req, account, resp)
	if err != nil {
		return diadem.Address{}, err
	}
	return diadem.UnmarshalAddressPB(resp.To), nil
}

func parseAddress(address string) (diadem.Address, error) {
	var addr diadem.Address
	addr, err := cli.ParseAddress(address)
	if err != nil {
		return addr, errors.Wrap(err, "failed to parse address")
	}
	//Resolve address if chainID does not match prefix
	if addr.ChainID != cli.TxFlags.ChainID {
		rpcClient := client.NewDAppChainRPCClient(cli.TxFlags.ChainID, cli.TxFlags.URI+"/rpc", cli.TxFlags.URI+"/query")
		mapperAddr, err := rpcClient.Resolve("addressmapper")
		if err != nil {
			return addr, errors.Wrap(err, "failed to resolve DAppChain Address Mapper address")
		}
		mapper := client.NewContract(rpcClient, mapperAddr.Local)
		mappedAccount, err := getMappedAccount(mapper, addr)
		if err != nil {
			return addr, fmt.Errorf("No account information found for %v", addr)
		}
		addr = mappedAccount
	}
	return addr, nil
}
