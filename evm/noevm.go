// +build !evm

package evm

import (
	"github.com/diademnetwork/diademchain"
	lvm "github.com/diademnetwork/diademchain/vm"
	dbm "github.com/tendermint/tendermint/libs/db"
)

var (
	LogEthDbBatch = true
)

// EVMEnabled indicates whether or not EVM integration is available
const EVMEnabled = false

func NewDiademVm(
	diademState diademchain.State,
	evmDB dbm.DB,
	eventHandler diademchain.EventHandler,
	receiptHandler diademchain.WriteReceiptHandler,
	createABM AccountBalanceManagerFactoryFunc,
	debug bool,
) lvm.VM {
	return nil
}

func AddDiademPrecompiles() {}
