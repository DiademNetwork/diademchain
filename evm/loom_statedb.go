// +build evm

package evm

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/state"
)

// DiademStateDB overrides some of the state.StateDB functions used to manage ETH balances to allow
// EVM contracts to seamlessly access ETH balances through the ethcoin Go contract.
type DiademStateDB struct {
	*state.StateDB
	abm *evmAccountBalanceManager
}

func newDiademStateDB(abm *evmAccountBalanceManager, root common.Hash, db state.Database) (*DiademStateDB, error) {
	sdb, err := state.New(root, db)
	if err != nil {
		return nil, err
	}
	return &DiademStateDB{
		StateDB: sdb,
		abm:     abm,
	}, nil
}

func (s *DiademStateDB) GetBalance(addr common.Address) *big.Int {
	return s.abm.GetBalance(addr)
}

func (s *DiademStateDB) SubBalance(address common.Address, amount *big.Int) {
	s.abm.SubBalance(address, amount)
}

func (s *DiademStateDB) AddBalance(address common.Address, amount *big.Int) {
	s.abm.AddBalance(address, amount)
}

func (s *DiademStateDB) SetBalance(address common.Address, amount *big.Int) {
	s.abm.SetBalance(address, amount)
}

func (s *DiademStateDB) Suicide(address common.Address) bool {
	s.SetBalance(address, common.Big0)
	return s.StateDB.Suicide(address)
}
