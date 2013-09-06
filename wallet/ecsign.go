package main

import (
	"bytes"
	"math/big"
	"crypto/rand"
	"crypto/ecdsa"
	"crypto/sha512"
)

type rand256 struct {
	*bytes.Buffer
}

func NewRand256(seed []byte, pk *ecdsa.PrivateKey) (res *rand256) {
	h := sha512.New()
	h.Write(seed)
	h.Write(pk.D.Bytes())

	// Even if RNG is broken, this should not hurt:
	var buf [64]byte
	rand.Read(buf[:])
	h.Write(buf[:])

	// Now turn the 64 bytes long result of the hash to the source of random bytes
	res = new(rand256)
	res.Buffer = bytes.NewBuffer(h.Sum(nil))

	return
}

func (rdr *rand256) Read(p []byte) (n int, err error) {
	return rdr.Buffer.Read(p)
}


func ecdsa_Sign(priv *ecdsa.PrivateKey, hash []byte) (r, s *big.Int, err error) {
	if *secrand {
		return ecdsa.Sign(NewRand256(hash, priv), priv, hash)
	} else {
		return ecdsa.Sign(rand.Reader, priv, hash)
	}
}
