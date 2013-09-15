package ecver

import (
	"math/big"
	"encoding/hex"
)

// NUM
type secp256k1_num_t struct {
	big.Int
}

func new_num_val(val *secp256k1_num_t) (res *secp256k1_num_t) {
	res = new(secp256k1_num_t)
	res.Int.Set(&val.Int)
	return
}

func new_num_from_string(s string, base int) (res *secp256k1_num_t) {
	res = new(secp256k1_num_t)
	res.Int.SetString(s, base)
	return
}

func (a *secp256k1_num_t) equal(b *secp256k1_num_t) bool {
	return a.Cmp(&b.Int)==0
}


func (num *secp256k1_num_t) String() string {
	return hex.EncodeToString(num.Bytes())
}

func (num *secp256k1_num_t) print(lab string) {
	println("NUM."+lab, num.String())
}

func (num *secp256k1_num_t) mask_bits(bits uint) {
	mask := new(big.Int).Lsh(BigInt1, bits)
	mask.Sub(mask, BigInt1)
	num.Int.And(&num.Int, mask)
}


func (a *secp256k1_num_t) split_exp() (r1, r2 *secp256k1_num_t) {
	var bnc1, bnc2, bnn2, bnt1, bnt2 secp256k1_num_t

	r1 = new(secp256k1_num_t)
	r2 = new(secp256k1_num_t)

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


func (a *secp256k1_num_t) split(bits uint) (rl, rh *secp256k1_num_t) {
	rl = new(secp256k1_num_t)
	rh = new(secp256k1_num_t)
	rl.Int.Set(&a.Int)
	rh.Int.Rsh(&rl.Int, bits)
	rl.mask_bits(bits)
	return
}

func (num *secp256k1_num_t) shift(bits uint) (res int) {
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


func (a *secp256k1_num_t) mod_inverse(m *secp256k1_num_t) (r *secp256k1_num_t) {
	r = new(secp256k1_num_t)
	r.ModInverse(&a.Int, &m.Int)
	return
}


func (a *secp256k1_num_t) mul(b *secp256k1_num_t) (r *secp256k1_num_t) {
	r = new(secp256k1_num_t)
	r.Mul(&a.Int, &b.Int)
	return
}

func (a *secp256k1_num_t) mod_mul(b *secp256k1_num_t, m *secp256k1_num_t) (r *secp256k1_num_t) {
	r = new(secp256k1_num_t)
	r.Mul(&a.Int, &b.Int)
	r.Mod(&r.Int, &m.Int)
	return
}
