package ecver

import (
	"math/big"
	"encoding/hex"
)

var (
	BigInt0 *big.Int = new(big.Int).SetInt64(0)
	BigInt1 *big.Int = new(big.Int).SetInt64(1)
)

// NUM
type num_t struct {
	big.Int
}

func new_num_val(val *num_t) (res *num_t) {
	res = new(num_t)
	res.Int.Set(&val.Int)
	return
}

func new_num_from_string(s string, base int) (res *num_t) {
	res = new(num_t)
	res.Int.SetString(s, base)
	return
}

func (fe *num_t) num() *num_t {
	return &num_t{Int:fe.Int}
}

func (a *num_t) equal(b *num_t) bool {
	return a.Cmp(&b.Int)==0
}


func (num *num_t) print(lab string) {
	println(lab, hex.EncodeToString(num.Bytes()))
}

func (num *num_t) mask_bits(bits uint) {
	mask := new(big.Int).Lsh(BigInt1, bits)
	mask.Sub(mask, BigInt1)
	num.Int.And(&num.Int, mask)
}


func (a *num_t) split_exp() (r1, r2 *num_t) {
	var bnc1, bnc2, bnn2, bnt1, bnt2 num_t

	r1 = new(num_t)
	r2 = new(num_t)

	bnn2.Int.Rsh(secp256k1.N, 1)

	bnc1.Mul(&a.Int, &a1b2.Int)
	bnc1.Add(&bnc1.Int, &bnn2.Int)
	bnc1.Div(&bnc1.Int, secp256k1.N)

	bnc2.Mul(&a.Int, &b1.Int)
	bnc2.Add(&bnc2.Int, &bnn2.Int)
	bnc2.Div(&bnc2.Int, secp256k1.N)

	bnt1.Mul(&bnc1.Int, &a1b2.Int)
	bnt2.Mul(&bnc2.Int, &a2.Int)
	bnt1.Add(&bnt1.Int, &bnt2.Int)
	r1.Sub(&a.Int, &bnt1.Int)

	bnt1.Mul(&bnc1.Int, &b1.Int)
	bnt2.Mul(&bnc2.Int, &a1b2.Int)
	r2.Sub(&bnt1.Int, &bnt2.Int)
	return
}


func (a *num_t) split(bits uint) (rl, rh *num_t) {
	rl = new(num_t)
	rh = new(num_t)
	rl.Int.Set(&a.Int)
	rh.Int.Rsh(&rl.Int, bits)
	rl.mask_bits(bits)
	return
}


func (num *num_t) rsh(bits uint) {
	num.Rsh(&num.Int, bits)
}

func (num *num_t) inc() {
	num.Add(&num.Int, BigInt1)
}

func (num *num_t) shift(bits uint) (res int) {
	mask := new(big.Int).Lsh(BigInt1, bits)
	mask.Sub(mask, BigInt1)
	res = int(new(big.Int).And(&num.Int, mask).Int64())
	if bits>0 {
		num.Rsh(&num.Int, bits)
	} else {
		num.Lsh(&num.Int, bits)
	}
	return
}


func (a *num_t) mod_inverse(m *num_t) (r *num_t) {
	r = new(num_t)
	r.ModInverse(&a.Int, &m.Int)
	return
}


func (a *num_t) mul(b *num_t) (r *num_t) {
	r = new(num_t)
	r.Mul(&a.Int, &b.Int)
	return
}

func (a *num_t) mod_mul(b *num_t, m *num_t) (r *num_t) {
	r = new(num_t)
	r.Mul(&a.Int, &b.Int)
	r.Mod(&r.Int, &m.Int)
	return
}
