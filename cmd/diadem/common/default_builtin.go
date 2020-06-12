// +build !gamechain

package common

import (
	godiademplugin "github.com/diademnetwork/go-diadem/plugin"
	"github.com/diademnetwork/diademchain/builtin/plugins/coin"
)

var builtinContracts []godiademplugin.Contract

func init() {
	builtinContracts = []godiademplugin.Contract{
		coin.Contract,
	}
}
