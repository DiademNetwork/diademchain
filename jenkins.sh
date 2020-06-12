#!/bin/bash

set -ex

PKG=github.com/diademnetwork/diademchain

# setup temp GOPATH
export GOPATH=/tmp/gopath-$BUILD_TAG
export
export PATH=$GOPATH:$PATH:/var/lib/jenkins/workspace/commongopath/bin:$GOPATH/bin

DIADEM_SRC=$GOPATH/src/$PKG
mkdir -p $DIADEM_SRC
rsync -r --delete . $DIADEM_SRC

if [[ "$OSTYPE" == "linux-gnu" ]]; then
export CGO_CFLAGS="-I/usr/local/include/leveldb"
export CGO_LDFLAGS="-L/usr/local/lib/ -L/usr/lib/x86_64-linux-gnu/ -lsnappy"
#elif [[ "$OSTYPE" == "darwin"* ]]; then #osx
fi

cd $DIADEM_SRC
make clean
make get_lint
make deps
make lint || true
make linterrors
make  # on OSX we don't need any C precompiles like cleveldb
make validators-tool
make tgoracle
make diademcoin_tgoracle
make dposv2_oracle
make plasmachain
make diadem-cleveldb
make plasmachain-cleveldb


export DIADEM_BIN=`pwd`/diadem
export DIADEM_VALIDATORS_TOOL=`pwd`/e2e/validators-tool

export GORACE="log_path=`pwd`/racelog"
#make diadem-race
#make test-race
make test
#make test-no-evm
make no-evm-tests
make test-app-store-race

#setup & run truffle tests
#cd e2e/tests/truffle
#yarn

#cd ../receipts
#bash ./run_truffle_tests.sh
