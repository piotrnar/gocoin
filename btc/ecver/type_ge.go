package ecver

import (
//	"math/big"
	"encoding/hex"
)

// GE
type secp256k1_ge_t struct {
    x, y secp256k1_fe_t
    infinity bool
}

func (r *secp256k1_ge_t) set_gej(a *secp256k1_gej_t) {
	var z2, z3 secp256k1_fe_t
	a.z.inv_s()
	a.z.sqr_p(&z2)
	a.z.mul_p(&z3, &z2)
	a.x.mul_s(&z2)
	a.y.mul_s(&z3)
	a.z.Set(BigInt1)
	r.infinity = a.infinity
	r.x = a.x
	r.y = a.y
}

func (ge *secp256k1_ge_t) print(lab string) {
	println("GE." + lab + ".x:", hex.EncodeToString(ge.x.Bytes()))
	println("GE." + lab + ".y:", hex.EncodeToString(ge.y.Bytes()))
}

func (a *secp256k1_ge_t) neg_p(rr *secp256k1_ge_t) {
	var r secp256k1_ge_t
	r.infinity = a.infinity
	r.x = a.x
	a.y.neg_p(&r.y)
	*rr = r
	return
}

func (a *secp256k1_ge_t) precomp(w int) (pre []secp256k1_ge_t) {
	pre = make([]secp256k1_ge_t, (1 << (uint(w)-2)))
	pre[0] = *a;
	var x, d, tmp secp256k1_gej_t
	x.set_ge(a)
	x.double_p(&d)
	for i:=1 ; i<len(pre); i++ {
		d.add_ge_p(&tmp, &pre[i-1])
		pre[i].set_gej(&tmp)
	}
	return
}
