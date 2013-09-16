package ecver

import (
	"math/big"
	"encoding/hex"
)

// FE
type secp256k1_fe_t struct {
	secp256k1_num_t
}

func new_fe_from_string(s string, base int) (r *secp256k1_fe_t) {
	r = new(secp256k1_fe_t)
	r.Int.SetString(s, base)
	return
}

func (fe *secp256k1_fe_t) String() string {
	return hex.EncodeToString(fe.Bytes())
}

func (a *secp256k1_fe_t) equal(b *secp256k1_fe_t) bool {
	return a.Cmp(&b.Int)==0
}

func (a *secp256k1_fe_t) mul_s(b *secp256k1_fe_t) {
	a.Mul(&a.Int, &b.Int)
	a.Mod(&a.Int, secp256k1.P)
	return
}

func (a *secp256k1_fe_t) mul_p(r *secp256k1_fe_t, b *secp256k1_fe_t) {
	r.Mul(&a.Int, &b.Int)
	r.Mod(&r.Int, secp256k1.P)
	return
}

func (a *secp256k1_fe_t) mul_int(b int) {
	var ad big.Int
	ad.Set(&a.Int)
	for i:=1; i<b; i++ {
		a.Add(&a.Int, &ad)
		if a.Cmp(secp256k1.P) >= 0 {
			a.Sub(&a.Int, secp256k1.P)
		}
	}
	return
}

func (a *secp256k1_fe_t) sqr_p(r *secp256k1_fe_t) {
	a.mul_p(r, a)
}

func (a *secp256k1_fe_t) sqr_s() {
	a.mul_s(a)
}

func (a *secp256k1_fe_t) neg_s() {
	a.Sub(secp256k1.P, &a.Int)
	return
}

func (a *secp256k1_fe_t) neg_p(r *secp256k1_fe_t) {
	r.Sub(secp256k1.P, &a.Int)
	return
}

func (a *secp256k1_fe_t) add_p(r *secp256k1_fe_t, b *secp256k1_fe_t) {
	r.Add(&a.Int, &b.Int)
	if r.Cmp(secp256k1.P) >= 0 {
		r.Sub(&r.Int, secp256k1.P)
	}
	return
}

func (a *secp256k1_fe_t) add_s(b *secp256k1_fe_t) {
	a.Add(&a.Int, &b.Int)
	if a.Cmp(secp256k1.P) >= 0 {
		a.Sub(&a.Int, secp256k1.P)
	}
	return
}

func (a *secp256k1_fe_t) inv_p(r *secp256k1_fe_t) {
	r.ModInverse(&a.Int, secp256k1.P)
	return
}

func (a *secp256k1_fe_t) inv_s() {
	a.ModInverse(&a.Int, secp256k1.P)
}
