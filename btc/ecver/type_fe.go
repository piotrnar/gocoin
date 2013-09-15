package ecver

import (
	"math/big"
	"encoding/hex"
)

// FE
type secp256k1_fe_t struct {
	secp256k1_num_t
}


func new_fe_from_string(s string, base int) (res *secp256k1_fe_t) {
	res = new(secp256k1_fe_t)
	res.Int.SetString(s, base)
	return
}

func (fe *secp256k1_fe_t) String() string {
	return hex.EncodeToString(fe.Bytes())
}


func (a *secp256k1_fe_t) equal(b *secp256k1_fe_t) bool {
	return a.Cmp(&b.Int)==0
}


func (a *secp256k1_fe_t) mul(b *secp256k1_fe_t) (res *secp256k1_fe_t) {
	res = new(secp256k1_fe_t)
	res.Mul(&a.Int, &b.Int)
	res.Mod(&res.Int, secp256k1.P)
	return
}

func (a *secp256k1_fe_t) mul_int(b int) {
	t := new(big.Int).Set(&a.Int)
	for i:=1; i<b; i++ {
		t.Add(t, &a.Int)
	}
	a.Mod(t, secp256k1.P)
	return
}


func (a *secp256k1_fe_t) sqr() (res *secp256k1_fe_t) {
	return a.mul(a)
}


func (a *secp256k1_fe_t) neg() (res *secp256k1_fe_t) {
	res = new(secp256k1_fe_t)
	res.Sub(secp256k1.P, &a.Int)
	return
}

func (a *secp256k1_fe_t) add(b *secp256k1_fe_t) (res *secp256k1_fe_t) {
	res = new(secp256k1_fe_t)
	res.Add(&a.Int, &b.Int)
	res.Mod(&res.Int, secp256k1.P)
	return
}


func (a *secp256k1_fe_t) inv() (r *secp256k1_fe_t) {
	r = new(secp256k1_fe_t)
	r.ModInverse(&a.Int, secp256k1.P)
	return
}
