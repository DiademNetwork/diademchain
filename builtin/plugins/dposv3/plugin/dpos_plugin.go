package main

import (
	"github.com/diademnetwork/go-diadem/plugin"
	"github.com/diademnetwork/diademchain/builtin/plugins/dposv3"
)

var Contract = dposv3.Contract

func main() {
	plugin.Serve(Contract)
}
