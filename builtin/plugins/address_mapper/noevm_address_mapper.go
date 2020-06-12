// +build !evm

package address_mapper

import (
	"crypto/ecdsa"

	"github.com/diademnetwork/go-diadem"
	amtypes "github.com/diademnetwork/go-diadem/builtin/types/address_mapper"
	"github.com/diademnetwork/go-diadem/plugin"
	contract "github.com/diademnetwork/go-diadem/plugin/contractpb"
)

type (
	InitRequest               = amtypes.AddressMapperInitRequest
	GetMappingRequest         = amtypes.AddressMapperGetMappingRequest
	GetMappingResponse        = amtypes.AddressMapperGetMappingResponse
	AddIdentityMappingRequest = amtypes.AddressMapperAddIdentityMappingRequest
	ListMappingResponse       = amtypes.AddressMapperListMappingResponse
	ListMappingRequest        = amtypes.AddressMapperListMappingRequest
)

type AddressMapper struct {
}

func (am *AddressMapper) Meta() (plugin.Meta, error) {
	return plugin.Meta{
		Name:    "addressmapper",
		Version: "0.1.0",
	}, nil
}

func (am *AddressMapper) Init(_ contract.Context, _ *InitRequest) error {
	return nil
}

func (am *AddressMapper) GetMapping(_ contract.StaticContext, _ *GetMappingRequest) (*GetMappingResponse, error) {
	return nil, nil
}

func (am *AddressMapper) AddIdentityMapping(_ contract.Context, _ *AddIdentityMappingRequest) error {
	return nil
}

func SignIdentityMapping(_, _ diadem.Address, _ *ecdsa.PrivateKey) ([]byte, error) {
	return nil, nil
}

var Contract plugin.Contract = contract.MakePluginContract(&AddressMapper{})
