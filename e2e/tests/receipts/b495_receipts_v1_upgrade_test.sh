#!/bin/bash

# This script tests backwards compatibility of EVM receipts handling (ReceiptsVersion: 1) between
# diadem build 495 and a newer build.
#
# It works by spinning up a cluster using build 495, sending some txs to the cluster Truffle tests,
# then shutting it down, and spinning it back up on the newer build.
#
# The diadem binaries are expected to be in the current directory, and should be named diadem-495, and
# diadem-497
#
# Example usage:
# ./b495_receipts_v1_upgrade_test.sh --v1 `pwd`/diadem-495 --v2 `pwd`/diadem-497

set -exo pipefail

TEST_DIR=`pwd`
TRUFFLE_DIR=`pwd`/../truffle
CLUSTER_RUNNING=false
CLUSTER=`pwd`/../cluster.sh
DIADEM_BIN_V1=`pwd`/diadem-495
DIADEM_BIN_V2=`pwd`/diadem-497

while [[ "$#" > 0 ]]; do case $1 in
  --v1) DIADEM_BIN_V1=$2; shift; shift;;
  --v2) DIADEM_BIN_V2=$2; shift; shift;;
  *) echo "Unknown parameter: $1"; shift; shift;;
esac; done

function stop_cluster {
    bash $CLUSTER --dir $TEST_DIR --stop
    CLUSTER_RUNNING=false
}

function cleanup {
    if [[ "$CLUSTER_RUNNING" == true ]]; then
        stop_cluster
    fi
}

trap cleanup EXIT

rm -rf $TEST_DIR/cluster

echo "Spinning up cluster with $DIADEM_BIN_V1"
DIADEM_BIN=$DIADEM_BIN_V1 \
bash $CLUSTER --init --dir $TEST_DIR --start --cfg `pwd`/b495_receipts_v1_diadem.yml

CLUSTER_RUNNING=true

pushd $TRUFFLE_DIR
CLUSTER_DIR=$TEST_DIR/cluster yarn test
popd

# give the nodes a bit of time to sync up
sleep 5

stop_cluster

# give the nodes a bit of time to shut down
sleep 5

for i in `seq 0 3`;
do
    pushd $TEST_DIR/cluster/${i}
    # stash logs for later
    mv diadem.log diadem1.log
    popd
done

# start the cluster up again with the new diadem build
echo "Spinning up cluster with $DIADEM_BIN_V2"
DIADEM_BIN=$DIADEM_BIN_V2 \
bash $CLUSTER --start
CLUSTER_RUNNING=true

# give the nodes a bit of time to sync up
sleep 5

# check the cluster is operational
pushd $TRUFFLE_DIR
CLUSTER_DIR=$TEST_DIR/cluster yarn test
popd

# give the nodes a bit of time digest
sleep 1
