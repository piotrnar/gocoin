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
	a.z = *a.z.inv()
	z2 := a.z.sqr()
	z3 := a.z.mul(z2)
	a.x = *a.x.mul(z2)
	a.y = *a.y.mul(z3)
	a.z.Set(BigInt1)
	r.infinity = a.infinity
	r.x = a.x
	r.y = a.y
}

func (ge *secp256k1_ge_t) print(lab string) {
	println("GE." + lab + ".x:", hex.EncodeToString(ge.x.Bytes()))
	println("GE." + lab + ".y:", hex.EncodeToString(ge.y.Bytes()))
}

func (a *secp256k1_ge_t) neg() (r *secp256k1_ge_t) {
	r = new(secp256k1_ge_t)
	r.infinity = a.infinity
	r.x = a.x
	r.y = *a.y.neg()
	return
}

func (a *secp256k1_ge_t) precomp(w int) (pre []secp256k1_ge_t) {
	pre = make([]secp256k1_ge_t, (1 << (uint(w)-2)))
	pre[0] = *a;
	var x secp256k1_gej_t
	x.set_ge(a)
	d := x.double()
	for i:=1 ; i<len(pre); i++ {
		pre[i].set_gej(d.add_ge(&pre[i-1]))
	}
	return
}
