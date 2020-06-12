package main

import (
	"github.com/diademnetwork/go-diadem/plugin"
	"github.com/diademnetwork/diademchain/builtin/plugins/coin"
)

var Contract = coin.Contract

func main() {
	plugin.Serve(Contract)
}
