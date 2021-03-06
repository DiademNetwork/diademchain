// +build evm

package oracle

import (
	"math/big"

	diadem "github.com/diademnetwork/go-diadem"
	"github.com/diademnetwork/go-diadem/auth"
	pctypes "github.com/diademnetwork/go-diadem/builtin/types/plasma_cash"
	"github.com/diademnetwork/go-diadem/client"
	ltypes "github.com/diademnetwork/go-diadem/types"
	"github.com/pkg/errors"
)

type DAppChainPlasmaClientConfig struct {
	ChainID  string
	WriteURI string
	ReadURI  string
	// Used to sign txs sent to Diadem DAppChain
	Signer auth.Signer
	// name of plasma cash contract on DAppChain
	ContractName string
}

type DAppChainPlasmaClient interface {
	Init() error
	CurrentPlasmaBlockNum() (*big.Int, error)
	PlasmaBlockAt(blockNum *big.Int) (*pctypes.PlasmaBlock, error)
	FinalizeCurrentPlasmaBlock() error
	GetPendingTxs() (*pctypes.PendingTxs, error)
	ProcessRequestBatch(requestBatch *pctypes.PlasmaCashRequestBatch) error
	GetRequestBatchTally() (*pctypes.PlasmaCashRequestBatchTally, error)
}

type DAppChainPlasmaClientImpl struct {
	DAppChainPlasmaClientConfig
	plasmaContract *client.Contract
	caller         diadem.Address
}

func (c *DAppChainPlasmaClientImpl) GetPendingTxs() (*pctypes.PendingTxs, error) {
	req := &pctypes.GetPendingTxsRequest{}
	resp := &pctypes.PendingTxs{}
	if _, err := c.plasmaContract.StaticCall("GetPendingTxs", req, c.caller, resp); err != nil {
		return nil, errors.Wrap(err, "failed to call GetPendingTxs")
	}

	return resp, nil
}

func (c *DAppChainPlasmaClientImpl) Init() error {
	dappClient := client.NewDAppChainRPCClient(c.ChainID, c.WriteURI, c.ReadURI)
	contractAddr, err := dappClient.Resolve(c.ContractName)
	if err != nil {
		return errors.Wrapf(err, "failed to resolve Plasma Go contract: %s address", c.ContractName)
	}
	c.plasmaContract = client.NewContract(dappClient, contractAddr.Local)
	c.caller = diadem.Address{
		ChainID: c.ChainID,
		Local:   diadem.LocalAddressFromPublicKey(c.Signer.PublicKey()),
	}
	return nil
}

func (c *DAppChainPlasmaClientImpl) CurrentPlasmaBlockNum() (*big.Int, error) {
	req := &pctypes.GetCurrentBlockRequest{}
	resp := &pctypes.GetCurrentBlockResponse{}
	if _, err := c.plasmaContract.StaticCall("GetCurrentBlockRequest", req, c.caller, resp); err != nil {
		return nil, errors.Wrap(err, "failed to call GetCurrentBlockRequest")
	}
	return resp.BlockHeight.Value.Int, nil
}

func (c *DAppChainPlasmaClientImpl) PlasmaBlockAt(blockNum *big.Int) (*pctypes.PlasmaBlock, error) {
	req := &pctypes.GetBlockRequest{
		BlockHeight: &ltypes.BigUInt{Value: *diadem.NewBigUInt(blockNum)},
	}
	resp := &pctypes.GetBlockResponse{}
	if _, err := c.plasmaContract.StaticCall("GetBlockRequest", req, c.caller, resp); err != nil {
		return nil, errors.Wrap(err, "failed to obtain plasma block from DAppChain")
	}
	if resp.Block == nil {
		return nil, errors.New("DAppChain returned empty plasma block")
	}
	return resp.Block, nil
}

func (c *DAppChainPlasmaClientImpl) FinalizeCurrentPlasmaBlock() error {
	breq := &pctypes.SubmitBlockToMainnetRequest{}
	if _, err := c.plasmaContract.Call("SubmitBlockToMainnet", breq, c.Signer, nil); err != nil {
		return errors.Wrap(err, "failed to commit SubmitBlockToMainnet tx")
	}
	return nil
}

func (c *DAppChainPlasmaClientImpl) GetRequestBatchTally() (*pctypes.PlasmaCashRequestBatchTally, error) {
	req := &pctypes.PlasmaCashGetRequestBatchTallyRequest{}
	resp := &pctypes.PlasmaCashRequestBatchTally{}
	if _, err := c.plasmaContract.StaticCall("GetRequestBatchTally", req, c.caller, resp); err != nil {
		return nil, errors.Wrap(err, "failed to get request batch tally")
	}

	return resp, nil
}

func (c *DAppChainPlasmaClientImpl) ProcessRequestBatch(requestBatch *pctypes.PlasmaCashRequestBatch) error {
	if _, err := c.plasmaContract.Call("ProcessRequestBatch", requestBatch, c.Signer, nil); err != nil {
		return errors.Wrap(err, "failed to commit process request batch tx")
	}

	return nil
}
