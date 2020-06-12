package vm

import (
	"errors"

	diadem "github.com/diademnetwork/go-diadem"
	"github.com/diademnetwork/diademchain"
)

type VM interface {
	Create(caller diadem.Address, code []byte, value *diadem.BigUInt) ([]byte, diadem.Address, error)
	Call(caller, addr diadem.Address, input []byte, value *diadem.BigUInt) ([]byte, error)
	StaticCall(caller, addr diadem.Address, input []byte) ([]byte, error)
	GetCode(addr diadem.Address) ([]byte, error)
}

type Factory func(diademchain.State) (VM, error)

type Manager struct {
	vms map[VMType]Factory
}

func NewManager() *Manager {
	return &Manager{
		vms: make(map[VMType]Factory),
	}
}

func (m *Manager) Register(typ VMType, fac Factory) {
	m.vms[typ] = fac
}

func (m *Manager) InitVM(typ VMType, state diademchain.State) (VM, error) {
	fac, ok := m.vms[typ]
	if !ok {
		return nil, errors.New("vm type not found")
	}

	return fac(state)
}
