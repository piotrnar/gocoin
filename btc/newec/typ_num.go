package newec

import (
	"fmt"
	"math/big"
	"encoding/hex"
)

var (
	BigInt1 *big.Int = new(big.Int).SetInt64(1)
)

type num_t struct {
	big.Int
}

func (a *num_t) print(label string) {
	fmt.Println(label, hex.EncodeToString(a.Bytes()))
}

func (a *num_t) big() *big.Int {
	return &a.Int
}

func (r *num_t) mod_mul(a, b, m *num_t) {
	r.Mul(&a.Int, &b.Int)
	r.Mod(&r.Int, &m.Int)
	return
}

func (r *num_t) mod_inv(a, b *num_t) {
	r.ModInverse(&a.Int, &b.Int)
	return
}

func (r *num_t) mod(a *num_t) {
	r.Mod(&r.Int, &a.Int)
	return
}

func (a *num_t) set_hex(s string) {
	a.SetString(s, 16)
}

func (a *num_t) set_bytes(b []byte) {
	a.SetBytes(b)
}

func (a *num_t) bytes() []byte {
	return a.Bytes()
}

func (num *num_t) mask_bits(bits uint) {
	mask := new(big.Int).Lsh(BigInt1, bits)
	mask.Sub(mask, BigInt1)
	num.Int.And(&num.Int, mask)
}

func (a *num_t) split_exp(r1, r2 *num_t) {
	var bnc1, bnc2, bnn2, bnt1, bnt2 num_t

	bnn2.Int.Rsh(secp256k1.order.big(), 1)

	bnc1.Mul(&a.Int, &secp256k1.a1b2.Int)
	bnc1.Add(&bnc1.Int, &bnn2.Int)
	bnc1.Div(&bnc1.Int, secp256k1.order.big())

	bnc2.Mul(&a.Int, &secp256k1.b1.Int)
	bnc2.Add(&bnc2.Int, &bnn2.Int)
	bnc2.Div(&bnc2.Int, secp256k1.order.big())

	bnt1.Mul(&bnc1.Int, &secp256k1.a1b2.Int)
	bnt2.Mul(&bnc2.Int, &secp256k1.a2.Int)
	bnt1.Add(&bnt1.Int, &bnt2.Int)
	r1.Sub(&a.Int, &bnt1.Int)

	bnt1.Mul(&bnc1.Int, &secp256k1.b1.Int)
	bnt2.Mul(&bnc2.Int, &secp256k1.a1b2.Int)
	r2.Sub(&bnt1.Int, &bnt2.Int)
}

func (a *num_t) split(rl, rh *num_t, bits uint) {
	rl.Int.Set(&a.Int)
	rh.Int.Rsh(&rl.Int, bits)
	rl.mask_bits(bits)
}

func (num *num_t) rsh(bits uint) {
	num.Rsh(&num.Int, bits)
}

func (num *num_t) inc() {
	num.Add(&num.Int, BigInt1)
}

func (num *num_t) rsh_x(bits uint) (res int) {
	res = int(new(big.Int).And(&num.Int, new(big.Int).SetUint64((1<<bits)-1)).Uint64())
	num.Rsh(&num.Int, bits)
	return
}

func (num *num_t) add(a *num_t, b *num_t) {
	num.Add(&a.Int, &b.Int)
}

func (num *num_t) sub(a *num_t, b *num_t) {
	num.Sub(&a.Int, &b.Int)
}

func (num *num_t) cmp(b *num_t) int {
	return num.Cmp(&b.Int)
}

func (num *num_t) set(b *num_t) {
	num.Set(&b.Int)
}

func (num *num_t) is_odd() bool {
	return num.Bit(0)!=0;
}


func (num *num_t) sign() int {
	return num.Sign()
}

func (num *num_t) get_bin(le int) ([]byte) {
	bts := num.Bytes()
	if len(bts) > le {
		panic("buffer too small")
	}
	if len(bts) == le {
		return bts
	}
	return append(make([]byte, le-len(bts)), bts...)
}
