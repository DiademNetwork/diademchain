// +build gamechain

package common

import (
	"github.com/diademnetwork/gamechain/battleground"
	godiademplugin "github.com/diademnetwork/go-diadem/plugin"
)

var builtinContracts []godiademplugin.Contract

func init() {
	builtinContracts = []godiademplugin.Contract{
		battleground.Contract,
	}
}
