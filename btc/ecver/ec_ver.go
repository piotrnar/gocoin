package ecver

import (
	"github.com/piotrnar/gocoin/btc"
)

var secp256k1 *btc.BitCurve = btc.S256()


func parse_public_key(kd []byte) (pkey *ge_t) {
	if len(kd)==65 {
		pkey = new(ge_t)
		pkey.x.SetBytes(kd[1:33])
		pkey.y.SetBytes(kd[33:65])
		return
	}

	if len(kd)==33 {
		pkey = new(ge_t)
		pkey.x.SetBytes(kd[1:33])
		//pkey.x.get_xo_p(&pkey.y, kd[0]==3)
		pkey.y.Set(btc.DecompressPoint(&pkey.x.Int, kd[0]==3))
		return
	}

	return
}


func Verify(kd []byte, sd []byte, h []byte) bool {
	pkey := parse_public_key(kd)
	if pkey == nil {
		return false
	}

	s, _ := btc.NewSignature(sd)
	if s == nil {
		return false
	}

	var sig sig_t
	var msg num_t

	sig.r.Set(s.R)
	sig.s.Set(s.S)

	msg.SetBytes(h)

	return sig.verify(pkey, &msg)
}
