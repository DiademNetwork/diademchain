package gateway

import (
	"time"

	diadem "github.com/diademnetwork/go-diadem"
	"github.com/diademnetwork/go-diadem/auth"
	tgtypes "github.com/diademnetwork/go-diadem/builtin/types/transfer_gateway"
	"github.com/diademnetwork/go-diadem/client"
	"github.com/pkg/errors"
)

type (
	ProcessEventBatchRequest           = tgtypes.TransferGatewayProcessEventBatchRequest
	GatewayStateRequest                = tgtypes.TransferGatewayStateRequest
	GatewayStateResponse               = tgtypes.TransferGatewayStateResponse
	ConfirmWithdrawalReceiptRequest    = tgtypes.TransferGatewayConfirmWithdrawalReceiptRequest
	PendingWithdrawalsRequest          = tgtypes.TransferGatewayPendingWithdrawalsRequest
	PendingWithdrawalsResponse         = tgtypes.TransferGatewayPendingWithdrawalsResponse
	MainnetEvent                       = tgtypes.TransferGatewayMainnetEvent
	MainnetDepositEvent                = tgtypes.TransferGatewayMainnetEvent_Deposit
	MainnetWithdrawalEvent             = tgtypes.TransferGatewayMainnetEvent_Withdrawal
	MainnetTokenDeposited              = tgtypes.TransferGatewayTokenDeposited
	MainnetTokenWithdrawn              = tgtypes.TransferGatewayTokenWithdrawn
	TokenKind                          = tgtypes.TransferGatewayTokenKind
	PendingWithdrawalSummary           = tgtypes.TransferGatewayPendingWithdrawalSummary
	UnverifiedContractCreatorsRequest  = tgtypes.TransferGatewayUnverifiedContractCreatorsRequest
	UnverifiedContractCreatorsResponse = tgtypes.TransferGatewayUnverifiedContractCreatorsResponse
	VerifyContractCreatorsRequest      = tgtypes.TransferGatewayVerifyContractCreatorsRequest
	UnverifiedContractCreator          = tgtypes.TransferGatewayUnverifiedContractCreator
	VerifiedContractCreator            = tgtypes.TransferGatewayVerifiedContractCreator
)

const (
	TokenKind_ERC721X  = tgtypes.TransferGatewayTokenKind_ERC721X
	TokenKind_ERC721   = tgtypes.TransferGatewayTokenKind_ERC721
	TokenKind_ERC20    = tgtypes.TransferGatewayTokenKind_ERC20
	TokenKind_ETH      = tgtypes.TransferGatewayTokenKind_ETH
	TokenKind_DiademCoin = tgtypes.TransferGatewayTokenKind_DIADEMCOIN
)

// DAppChainGateway is a partial client-side binding of the Gateway Go contract
type DAppChainGateway struct {
	Address diadem.Address
	// Timestamp of the last successful response from the DAppChain
	LastResponseTime time.Time

	contract *client.Contract
	caller   diadem.Address
	logger   *diadem.Logger
	signer   auth.Signer
}

func ConnectToDAppChainDiademCoinGateway(
	diademClient *client.DAppChainRPCClient, caller diadem.Address, signer auth.Signer,
	logger *diadem.Logger,
) (*DAppChainGateway, error) {
	gatewayAddr, err := diademClient.Resolve("diademcoin-gateway")
	if err != nil {
		return nil, errors.Wrap(err, "failed to resolve Gateway Go contract address")
	}

	return &DAppChainGateway{
		Address:          gatewayAddr,
		LastResponseTime: time.Now(),
		contract:         client.NewContract(diademClient, gatewayAddr.Local),
		caller:           caller,
		signer:           signer,
		logger:           logger,
	}, nil
}

func ConnectToDAppChainGateway(
	diademClient *client.DAppChainRPCClient, caller diadem.Address, signer auth.Signer,
	logger *diadem.Logger,
) (*DAppChainGateway, error) {
	gatewayAddr, err := diademClient.Resolve("gateway")
	if err != nil {
		return nil, errors.Wrap(err, "failed to resolve Gateway Go contract address")
	}

	return &DAppChainGateway{
		Address:          gatewayAddr,
		LastResponseTime: time.Now(),
		contract:         client.NewContract(diademClient, gatewayAddr.Local),
		caller:           caller,
		signer:           signer,
		logger:           logger,
	}, nil
}

func (gw *DAppChainGateway) LastMainnetBlockNum() (uint64, error) {
	var resp GatewayStateResponse
	if _, err := gw.contract.StaticCall("GetState", &GatewayStateRequest{}, gw.caller, &resp); err != nil {
		gw.logger.Error("failed to retrieve state from Gateway contract on DAppChain", "err", err)
		return 0, err
	}
	gw.LastResponseTime = time.Now()
	return resp.State.LastMainnetBlockNum, nil
}

func (gw *DAppChainGateway) ProcessEventBatch(events []*MainnetEvent) error {
	// TODO: limit max message size to under 1MB
	req := &ProcessEventBatchRequest{
		Events: events,
	}
	if _, err := gw.contract.Call("ProcessEventBatch", req, gw.signer, nil); err != nil {
		gw.logger.Error("failed to commit ProcessEventBatch tx", "err", err)
		return err
	}
	gw.LastResponseTime = time.Now()
	return nil
}

func (gw *DAppChainGateway) PendingWithdrawals(mainnetGatewayAddr diadem.Address) ([]*PendingWithdrawalSummary, error) {
	req := &PendingWithdrawalsRequest{
		MainnetGateway: mainnetGatewayAddr.MarshalPB(),
	}
	resp := PendingWithdrawalsResponse{}
	if _, err := gw.contract.StaticCall("PendingWithdrawals", req, gw.caller, &resp); err != nil {
		gw.logger.Error("failed to fetch pending withdrawals from DAppChain", "err", err)
		return nil, err
	}
	gw.LastResponseTime = time.Now()
	return resp.Withdrawals, nil
}

func (gw *DAppChainGateway) ConfirmWithdrawalReceipt(req *ConfirmWithdrawalReceiptRequest) error {
	_, err := gw.contract.Call("ConfirmWithdrawalReceipt", req, gw.signer, nil)
	if err != nil {
		return err
	}
	gw.LastResponseTime = time.Now()
	return nil
}

func (gw *DAppChainGateway) ConfirmWithdrawalReceiptV2(req *ConfirmWithdrawalReceiptRequest) error {
	_, err := gw.contract.Call("ConfirmWithdrawalReceiptV2", req, gw.signer, nil)
	if err != nil {
		return err
	}
	gw.LastResponseTime = time.Now()
	return nil
}

func (gw *DAppChainGateway) UnverifiedContractCreators() ([]*UnverifiedContractCreator, error) {
	req := &UnverifiedContractCreatorsRequest{}
	resp := UnverifiedContractCreatorsResponse{}
	if _, err := gw.contract.StaticCall("UnverifiedContractCreators", req, gw.caller, &resp); err != nil {
		gw.logger.Error("failed to fetch pending contract mappings from DAppChain", "err", err)
		return nil, err
	}
	gw.LastResponseTime = time.Now()
	return resp.Creators, nil
}

func (gw *DAppChainGateway) VerifyContractCreators(verifiedCreators []*VerifiedContractCreator) error {
	req := &VerifyContractCreatorsRequest{
		Creators: verifiedCreators,
	}
	_, err := gw.contract.Call("VerifyContractCreators", req, gw.signer, nil)
	if err != nil {
		return err
	}
	gw.LastResponseTime = time.Now()
	return nil
}
