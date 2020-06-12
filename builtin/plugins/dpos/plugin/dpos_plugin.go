package main

import (
	"github.com/diademnetwork/go-diadem/plugin"
	"github.com/diademnetwork/diademchain/builtin/plugins/dpos"
)

var Contract = dpos.Contract

func main() {
	plugin.Serve(Contract)
}
