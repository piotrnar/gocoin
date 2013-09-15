package ecver

import (
	"github.com/piotrnar/gocoin/btc"
)

var secp256k1 *btc.BitCurve = btc.S256()

func Verify(kd []byte, sd []byte, h []byte) bool {
	pk, e := btc.NewPublicKey(kd)
	if e != nil {
		return false
	}
	s, e := btc.NewSignature(sd)
	if e != nil {
		return false
	}

	var sig secp256k1_ecdsa_sig_t
	var pkey secp256k1_ge_t
	var msg secp256k1_num_t

	sig.r.Set(s.R)
	sig.s.Set(s.S)

	pkey.x.Set(pk.X)
	pkey.y.Set(pk.Y)

	msg.SetBytes(h)

	return sig.verify(&pkey, &msg)
}
