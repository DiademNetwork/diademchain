package main

import (
	"github.com/diademnetwork/go-diadem/plugin"
	"github.com/diademnetwork/diademchain/builtin/plugins/dposv2"
)

var Contract = dposv2.Contract

func main() {
	plugin.Serve(Contract)
}
