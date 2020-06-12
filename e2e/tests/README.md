`cluster.sh` can be used to spin up a local 4-node cluster for testing.


## Setup

```bash
# in repo root...
make diadem
make validators-tool
export DIADEM_BIN=`pwd`/diadem
export DIADEM_VALIDATORS_TOOL=`pwd`/e2e/validators-tool

# setup truffle
cd e2e/tests/truffle
yarn
```


## Testing

```bash
cd e2e/tests/receipts
./run_truffle_tests.sh
```
