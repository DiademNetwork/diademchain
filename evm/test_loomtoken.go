// +build evm

package evm

import (
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/gogo/protobuf/proto"
	lp "github.com/diademnetwork/go-diadem"
	lvm "github.com/diademnetwork/diademchain/vm"
	"github.com/stretchr/testify/require"
)

// Pseudo code
// diademToken = new DiademToken()
// delegateCallToken = new DelegateCallToken()
// transferGateway = new (diademToken, delegateCallToken, 0)
// diademToken.transfer( transferGateway, 10)  -> returns true
func testDiademTokens(t *testing.T, vm lvm.VM, caller lp.Address) {
	diademData := getContractData("./testdata/DiademToken.json")
	delegateCallData := getContractData("./testdata/DelegateCallToken.json")

	addrDiademToken := deployContract(t, vm, caller, snipOx(diademData.Bytecode), snipOx(diademData.DeployedBytecode))
	addrDelegateCallToken := deployContract(t, vm, caller, snipOx(delegateCallData.Bytecode), snipOx(delegateCallData.DeployedBytecode))
	addrTransferGateway := createTransferGateway(t, vm, caller, addrDiademToken, addrDelegateCallToken)
	_ = callTransfer(t, vm, caller, addrDiademToken, addrTransferGateway, uint64(10))
}

func createTransferGateway(t *testing.T, vm lvm.VM, caller, diademAdr, delAdr lp.Address) lp.Address {
	var empty []byte
	transferGatewayData := getContractData("./testdata/TransferGateway.json")
	inParams := evmParamsB(common.Hex2Bytes(snipOx(transferGatewayData.Bytecode)), diademAdr.Local, delAdr.Local, empty)

	res, addr, err := vm.Create(caller, inParams, lp.NewBigUIntFromInt(0))
	require.NoError(t, err)

	output := lvm.DeployResponseData{}
	err = proto.Unmarshal(res, &output)
	require.NoError(t, err)
	if !checkEqual(output.Bytecode, common.Hex2Bytes(snipOx(transferGatewayData.DeployedBytecode))) {
		t.Error("create transfer Gateway did not return deployed bytecode")
	}
	return addr
}

func callTransfer(t *testing.T, vm lvm.VM, caller, contractAddr, addr2 lp.Address, amount uint64) bool {
	inParams := evmParams("transfer(address,uint256)", addr2.Local, uint64ToByte(amount))

	_, err := vm.Call(caller, contractAddr, inParams, lp.NewBigUIntFromInt(0))

	require.Nil(t, err)
	return false
}
