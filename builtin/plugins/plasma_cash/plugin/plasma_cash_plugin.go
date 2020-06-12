package main

import (
	"github.com/diademnetwork/go-diadem/plugin"
	"github.com/diademnetwork/diademchain/builtin/plugins/plasma_cash"
)

var Contract = plasma_cash.Contract

func main() {
	plugin.Serve(Contract)
}
