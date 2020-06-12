// +build evm

package auth

import (
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/diademnetwork/go-diadem/common/evmcompat"
	sha3 "github.com/miguelmota/go-solidity-sha3"
	"github.com/pkg/errors"
)

func verifySolidity66Byte(tx SignedTx) ([]byte, error) {
	ethAddr, err := evmcompat.RecoverAddressFromTypedSig(sha3.SoliditySHA3(tx.Inner), tx.Signature)
	if err != nil {
		return nil, errors.Wrap(err, "verify solidity key")
	}

	return ethAddr.Bytes(), nil
}

func verifyTron(tx SignedTx) ([]byte, error) {
	publicKeyBytes, err := crypto.Ecrecover(sha3.SoliditySHA3(tx.Inner), tx.Signature)
	if err != nil {
		return nil, err
	}
	publicKey, err := crypto.UnmarshalPubkey(publicKeyBytes)
	if err != nil {
		return nil, err
	}
	return crypto.PubkeyToAddress(*publicKey).Bytes(), nil
}
