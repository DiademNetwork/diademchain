GOLint = \
	github.com/golangci/golangci-lint/cmd/golangci-lint \

PKG = github.com/diademnetwork/diademchain
PKG_GAMECHAIN = github.com/diademnetwork/gamechain
PKG_BATTLEGROUND = $(PKG_GAMECHAIN)/battleground

PROTOC = protoc --plugin=./protoc-gen-gogo -Ivendor -I$(GOPATH)/src -I/usr/local/include

PLUGIN_DIR = $(GOPATH)/src/github.com/diademnetwork/go-diadem
GOLANG_PROTOBUF_DIR = $(GOPATH)/src/github.com/golang/protobuf
GOGO_PROTOBUF_DIR = $(GOPATH)/src/github.com/gogo/protobuf
GO_ETHEREUM_DIR = $(GOPATH)/src/github.com/ethereum/go-ethereum
SSHA3_DIR = $(GOPATH)/src/github.com/miguelmota/go-solidity-sha3
HASHICORP_DIR = $(GOPATH)/src/github.com/hashicorp/go-plugin
LEVIGO_DIR = $(GOPATH)/src/github.com/jmhodges/levigo
GAMECHAIN_DIR = $(GOPATH)/src/github.com/diademnetwork/gamechain

# NOTE: To build on Jenkins using a custom go-diadem branch update the `deps` target below to checkout
#       that branch, you only need to update GO_DIADEM_GIT_REV if you wish to lock the build to a
#       specific commit.
GO_DIADEM_GIT_REV = HEAD
# diademnetwork/go-ethereum diademchain branch
ETHEREUM_GIT_REV = 1fb6138d017a4309105d91f187c126cf979c93f9
# use go-plugin we get 'timeout waiting for connection info' error
HASHICORP_GIT_REV = f4c3476bd38585f9ec669d10ed1686abd52b9961
LEVIGO_GIT_REV = c42d9e0ca023e2198120196f842701bb4c55d7b9

BUILD_DATE = `date -Iseconds`
GIT_SHA = `git rev-parse --verify HEAD`
GO_DIADEM_GIT_SHA = `cd ${PLUGIN_DIR} && git rev-parse --verify ${GO_DIADEM_GIT_REV}`
ETHEREUM_GIT_SHA = `cd ${GO_ETHEREUM_DIR} && git rev-parse --verify ${ETHEREUM_GIT_REV}`
HASHICORP_GIT_SHA = `cd ${HASHICORP_DIR} && git rev-parse --verify ${HASHICORP_GIT_REV}`
GAMECHAIN_GIT_SHA = `cd ${GAMECHAIN_DIR} && git rev-parse --verify HEAD`

GOFLAGS_BASE = -X $(PKG).Build=$(BUILD_NUMBER) -X $(PKG).GitSHA=$(GIT_SHA) -X $(PKG).GoDiademGitSHA=$(GO_DIADEM_GIT_SHA) -X $(PKG).EthGitSHA=$(ETHEREUM_GIT_SHA) -X $(PKG).HashicorpGitSHA=$(HASHICORP_GIT_SHA)
GOFLAGS = -tags "evm" -ldflags "$(GOFLAGS_BASE)"
GOFLAGS_GAMECHAIN_BASE = -X $(PKG_BATTLEGROUND).BuildDate=$(BUILD_DATE) -X $(PKG_BATTLEGROUND).BuildGitSha=$(GAMECHAIN_GIT_SHA) -X $(PKG_BATTLEGROUND).BuildNumber=$(BUILD_NUMBER)
GOFLAGS_GAMECHAIN = -tags "evm gamechain" -ldflags "$(GOFLAGS_BASE) $(GOFLAGS_GAMECHAIN_BASE)"
GOFLAGS_PLASMACHAIN = -tags "evm plasmachain" -ldflags "$(GOFLAGS_BASE) -X $(PKG).BuildVariant=plasmachain"
GOFLAGS_PLASMACHAIN_CLEVELDB = -tags "evm plasmachain gcc" -ldflags "$(GOFLAGS_BASE) -X $(PKG).BuildVariant=plasmachain"
GOFLAGS_CLEVELDB = -tags "evm gcc" -ldflags "$(GOFLAGS_BASE)"
GOFLAGS_GAMECHAIN_CLEVELDB = -tags "evm gamechain gcc" -ldflags "$(GOFLAGS_BASE) $(GOFLAGS_GAMECHAIN_BASE)"
GOFLAGS_NOEVM = -ldflags "$(GOFLAGS_BASE)"

WINDOWS_BUILD_VARS = CC=x86_64-w64-mingw32-gcc CGO_ENABLED=1 GOOS=windows GOARCH=amd64 BIN_EXTENSION=.exe

E2E_TESTS_TIMEOUT = 28m

.PHONY: all clean test install get_lint update_lint deps proto builtin oracles tgoracle diademcoin_tgoracle pcoracle dposv2_oracle plasmachain-cleveldb diadem-cleveldb lint

all: diadem builtin

oracles: tgoracle pcoracle

builtin: contracts/coin.so.1.0.0 contracts/dpos.so.1.0.0 contracts/dpos.so.2.0.0 contracts/dpos.so.3.0.0 contracts/plasmacash.so.1.0.0

contracts/coin.so.1.0.0:
	go build -buildmode=plugin -o $@ $(GOFLAGS) $(PKG)/builtin/plugins/coin/plugin

contracts/dpos.so.1.0.0:
	go build -buildmode=plugin -o $@ $(GOFLAGS) $(PKG)/builtin/plugins/dpos/plugin

contracts/dpos.so.2.0.0:
	go build -buildmode=plugin -o $@ $(GOFLAGS) $(PKG)/builtin/plugins/dposv2/plugin

contracts/dpos.so.3.0.0:
	go build -buildmode=plugin -o $@ $(GOFLAGS) $(PKG)/builtin/plugins/dposv3/plugin

contracts/plasmacash.so.1.0.0:
	go build -buildmode=plugin -o $@ $(GOFLAGS) $(PKG)/builtin/plugins/plasma_cash/plugin

tgoracle:
	go build $(GOFLAGS) -o $@ $(PKG)/cmd/$@

diademcoin_tgoracle:
	go build $(GOFLAGS) -o $@ $(PKG)/cmd/$@

pcoracle:
	go build $(GOFLAGS) -o $@ $(PKG)/cmd/$@

dposv2_oracle:
	go build $(GOFLAGS) -o $@ $(PKG)/cmd/$@

diadem: proto
	go build $(GOFLAGS) $(PKG)/cmd/$@

diadem-windows:
	$(WINDOWS_BUILD_VARS) make diadem

gamechain: proto
	go build $(GOFLAGS_GAMECHAIN) -o gamechain$(BIN_EXTENSION) $(PKG)/cmd/diadem

gamechain-cleveldb: proto  c-leveldb
	go build $(GOFLAGS_GAMECHAIN_CLEVELDB) -o gamechain$(BIN_EXTENSION) $(PKG)/cmd/diadem

gamechain-windows: proto
	$(WINDOWS_BUILD_VARS) make gamechain

diadem-cleveldb: proto c-leveldb
	go build $(GOFLAGS_CLEVELDB) -o $@ $(PKG)/cmd/diadem

plasmachain: proto
	go build $(GOFLAGS_PLASMACHAIN) -o $@ $(PKG)/cmd/diadem

plasmachain-cleveldb: proto c-leveldb
	go build $(GOFLAGS_PLASMACHAIN_CLEVELDB) -o $@ $(PKG)/cmd/diadem

plasmachain-windows:
	$(WINDOWS_BUILD_VARS) make plasmachain

diadem-race: proto
	go build -race $(GOFLAGS) -o diadem-race $(PKG)/cmd/diadem

install: proto
	go install $(GOFLAGS) $(PKG)/cmd/diadem

protoc-gen-gogo:
	go build github.com/gogo/protobuf/protoc-gen-gogo

%.pb.go: %.proto protoc-gen-gogo
	if [ -e "protoc-gen-gogo.exe" ]; then mv protoc-gen-gogo.exe protoc-gen-gogo; fi
	$(PROTOC) --gogo_out=$(GOPATH)/src $(PKG)/$<

get_lint:
	@echo "--> Installing lint"
	chmod +x get_lint.sh
	./get_lint.sh

update_lint:
	@echo "--> Updating lint"
	./get_lint.sh

lint:
	cd $(GOPATH)/bin && chmod +x golangci-lint
	cd $(GOPATH)/src/github.com/diademnetwork/diademchain
	@golangci-lint run | tee lintreport

linterrors:		
	chmod +x parselintreport.sh
	./parselintreport.sh

proto: registry/registry.pb.go

c-leveldb:
	go get github.com/jmhodges/levigo
	cd $(LEVIGO_DIR) && git checkout master && git pull && git checkout $(LEVIGO_GIT_REV)

$(PLUGIN_DIR):
	git clone -q git@github.com:diademnetwork/go-diadem.git $@

$(GO_ETHEREUM_DIR):
	git clone -q git@github.com:diademnetwork/go-ethereum.git $@

$(SSHA3_DIR):
	git clone -q git@github.com:diademnetwork/go-solidity-sha3.git $@

validators-tool:
	go build -o e2e/validators-tool $(PKG)/e2e/cmd

deps: $(PLUGIN_DIR) $(GO_ETHEREUM_DIR) $(SSHA3_DIR)
	go get \
		golang.org/x/crypto/ed25519 \
		google.golang.org/grpc \
		github.com/gogo/protobuf/gogoproto \
        github.com/gogo/protobuf/proto \
		github.com/hashicorp/go-plugin \
		github.com/spf13/cobra \
		github.com/spf13/pflag \
		github.com/go-kit/kit/log \
		github.com/grpc-ecosystem/go-grpc-prometheus \
		github.com/prometheus/client_golang/prometheus \
		github.com/go-kit/kit/log \
		github.com/BurntSushi/toml \
		github.com/ulule/limiter \
		github.com/diademnetwork/mamamerkle \
		golang.org/x/sys/cpu \
		github.com/diademnetwork/yubihsm-go \
		github.com/gorilla/websocket \
		github.com/phonkee/go-pubsub \
		github.com/inconshreveable/mousetrap \
		github.com/posener/wstest

	# for when you want to reference a different branch of go-diadem
	#cd $(PLUGIN_DIR) && git checkout auto-enable-pending-feature && git pull origin auto-enable-pending-feature
	cd $(GOLANG_PROTOBUF_DIR) && git checkout v1.1.0
	cd $(GOGO_PROTOBUF_DIR) && git checkout v1.1.1
	cd $(GO_ETHEREUM_DIR) && git checkout master && git pull && git checkout $(ETHEREUM_GIT_REV)
	cd $(HASHICORP_DIR) && git checkout $(HASHICORP_GIT_REV)
	# fetch vendored packages
	dep ensure -vendor-only

#TODO we should turn back vet on, it broke when we upgraded go versions
test: proto
	go test  -failfast -timeout $(E2E_TESTS_TIMEOUT) -v -vet=off $(GOFLAGS) $(PKG)/...

test-race: proto
	go test -race -failfast -timeout $(E2E_TESTS_TIMEOUT) -v -vet=off $(GOFLAGS) $(PKG)/...

test-no-evm: proto
	go test -failfast -timeout $(E2E_TESTS_TIMEOUT) -v -vet=off $(GOFLAGS_NOEVM) $(PKG)/...

# Only builds the tests with the EVM disabled, but doesn't actually run them.
no-evm-tests: proto
	go test -failfast -v -vet=off $(GOFLAGS_NOEVM) -run nothing $(PKG)/...

test-e2e:
	go test -failfast -timeout $(E2E_TESTS_TIMEOUT) -v -vet=off $(PKG)/e2e

test-e2e-race:
	go test -race -failfast -timeout $(E2E_TESTS_TIMEOUT) -v -vet=off $(PKG)/e2e

test-app-store-race:
	go test -race -timeout 2m -failfast -v $(GOFLAGS) $(PKG)/store -run TestMultiReaderIAVLStore
	#go test -race -timeout 2m -failfast -v $(GOFLAGS) $(PKG)/store -run TestIAVLStoreTestSuite

vet:
	go vet ./...

vet-evm:
	go vet -tags evm ./...

clean:
	go clean
	rm -f \
		diadem \
		protoc-gen-gogo \
		contracts/coin.so.1.0.0 \
		contracts/dpos.so.1.0.0 \
		contracts/dpos.so.2.0.0 \
		contracts/dpos.so.3.0.0 \
		contracts/plasmacash.so.1.0.0 \
		pcoracle
