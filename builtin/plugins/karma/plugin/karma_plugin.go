package main

import (
	"github.com/diademnetwork/go-diadem/plugin"
	"github.com/diademnetwork/diademchain/builtin/plugins/karma"
)

var Contract = karma.Contract

func main() {
	plugin.Serve(Contract)
}
