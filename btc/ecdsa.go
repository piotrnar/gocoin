package btc

import (
	"bytes"
	"math/big"
	"sync/atomic"
	"crypto/rand"
	"crypto/ecdsa"
	"crypto/sha512"
	"github.com/piotrnar/gocoin/btc/newec"
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
	return newec.Verify(kd, sd, hash)
}


// Signing...
type rand256 struct {
	*bytes.Buffer
}

func (rdr *rand256) Read(p []byte) (n int, err error) {
	return rdr.Buffer.Read(p)
}


func EcdsaSign(priv *ecdsa.PrivateKey, hash []byte) (r, s *big.Int, err error) {
	h := sha512.New()
	h.Write(hash)
	h.Write(priv.D.Bytes())

	// Even if RNG is broken, this should not hurt:
	var buf [64]byte
	rand.Read(buf[:])
	h.Write(buf[:])

	// Now turn the 64 bytes long result of the hash to the source of random bytes
	radrd := new(rand256)
	radrd.Buffer = bytes.NewBuffer(h.Sum(nil))

	return ecdsa.Sign(radrd, priv, hash)
}
