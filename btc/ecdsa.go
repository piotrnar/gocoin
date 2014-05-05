package btc

import (
	"errors"
	"math/big"
	"sync/atomic"
	"github.com/piotrnar/gocoin/secp256k1"
)

var (
	EcdsaVerifyCnt uint64
	EC_Verify func(k, s, h []byte) bool
)

func EcdsaVerify(kd []byte, sd []byte, hash []byte) bool {
	atomic.AddUint64(&EcdsaVerifyCnt, 1)
	if EC_Verify!=nil {
		return EC_Verify(kd, sd, hash)
	}
	return secp256k1.Verify(kd, sd, hash)
}


func EcdsaSign(priv, hash []byte) (r, s *big.Int, err error) {
	var sig secp256k1.Signature
	var sec, msg, nonce secp256k1.Number
	var nv [32]byte

	sec.SetBytes(priv)
	msg.SetBytes(hash)

	ShaHash(hash, nv[:])
	for {
		nonce.SetBytes(nv[:])
		if nonce.Sign()>0 && nonce.Cmp(&secp256k1.TheCurve.Order.Int)<0 {
			break
		}
		ShaHash(nv[:], nv[:])
	}

	if sig.Sign(&sec, &msg, &nonce, nil)!=1 {
		err = errors.New("ESCDS Sign error()")
	}
	return &sig.R.Int, &sig.S.Int, nil
}
