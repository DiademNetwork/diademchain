package evm

import (
	"github.com/diademnetwork/go-diadem"
)

// AccountBalanceManager can be implemented to override the builtin account balance management in the EVM.
type AccountBalanceManager interface {
	GetBalance(addr diadem.Address) (*diadem.BigUInt, error)
	Transfer(from, to diadem.Address, amount *diadem.BigUInt) error
	AddBalance(addr diadem.Address, amount *diadem.BigUInt) error
	SubBalance(addr diadem.Address, amount *diadem.BigUInt) error
	SetBalance(addr diadem.Address, amount *diadem.BigUInt) error
}

type AccountBalanceManagerFactoryFunc func(readOnly bool) AccountBalanceManager
