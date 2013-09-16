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

func (fe *secp256k1_fe_t) num() *secp256k1_num_t {
	return &secp256k1_num_t{Int:fe.Int}
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

/*
func (a *secp256k1_fe_t) sqrt_p(r *secp256k1_fe_t) {
	var a2, a3, a6, a12, a15, a30, a60, a120, a240 secp256k1_fe_t
	var a255, a510, a750, a780, a1020, a1022, a1023 secp256k1_fe_t
	a.sqr_p(&a2)
	a2.mul_p(&a3, a)
	a3.sqr_p(&a6)
	a6.sqr_p(&a12)
	a12.mul_p(&a15, &a3)
	a15.sqr_p(&a30)
	a30.sqr_p(&a60)
	a60.sqr_p(&a120)
	a120.sqr_p(&a240)
	a240.mul_p(&a255, &a15)
	a255.sqr_p(&a510)
	a510.mul_p(&a750, &a240)
	a750.mul_p(&a780, &a30)
	a510.sqr_p(&a1020)
	a1020.mul_p(&a1022, &a2)
	a1022.mul_p(&a1023, a)
	x := a15

	for i:=0; i<21; i++ {
		for j:=0; j<10; j++ {
			x.sqr_s()
		}
		x.mul_s(&a1023)
	}
	for i:=0; i<10; i++ {
		x.sqr_s()
	}
	x.mul_s(&a1022)
	for i:=0; i<2; i++ {
		for j:=0; j<10; j++ {
			x.sqr_s()
		}
		x.mul_s(&a1023)
	}
	for i:=0; i<10; i++ {
		x.sqr_s()
	}
	x.mul_p(r, &a780)
	return
}

func (x *secp256k1_fe_t) get_xo_p(r *secp256k1_fe_t, odd bool) {
	var x2, x3, c secp256k1_fe_t
	x.sqr_p(&x2)
	x.mul_p(&x3, &x2)
	c.SetUint64(7)
	c.add_s(&x3)

	c.sqrt_p(r)
	if (r.Bit(0)!=0) != odd {
		r.neg_s()
	}
	return
}
*/