package btc

import (
	"sync/atomic"
	"crypto/ecdsa"
)

var (
	EcdsaVerifyCnt uint64
	EC_Verify func(k, s, h []byte) bool
)

// Use crypto/ecdsa
func GoVerify(kd []byte, sd []byte, h []byte) bool {
	pk, e := NewPublicKey(kd)
	if e != nil {
		return false
	}
	s, e := NewSignature(sd)
	if e != nil {
		return false
	}
	return ecdsa.Verify(&pk.PublicKey, h, s.R, s.S)
}


func EcdsaVerify(kd []byte, sd []byte, hash []byte) bool {
	atomic.AddUint64(&EcdsaVerifyCnt, 1)
	if EC_Verify!=nil {
		return EC_Verify(kd, sd, hash)
	}
	return GoVerify(kd, sd, hash)
}
