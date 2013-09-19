package newec

/*
void bytes2bn(void *out, void *bytes, int len);
int bn2bytes(void *bn, void *to);
*/
import "C"

import (
	"fmt"
	"unsafe"
	"math/big"
	"encoding/hex"
)

var (
	BigInt0 *big.Int = new(big.Int).SetInt64(0)
	BigInt1 *big.Int = new(big.Int).SetInt64(1)
)

type num_t struct {
	big.Int
}

func (a *num_t) init() {
}

func (a *num_t) free() {
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


/*Temporary functions*/
func (a *num_t) set_bn(bn unsafe.Pointer) {
	var buf [512]byte
	n := C.bn2bytes(bn, unsafe.Pointer(&buf[0]))
	a.SetBytes(buf[:int(n)])
}

func (a *num_t) get_bn() unsafe.Pointer {
	res := make([]byte, 64)
	dat := a.Bytes()
	if len(dat)==0 {
		dat = []byte{0}
	}
	C.bytes2bn(unsafe.Pointer(&res[0]), unsafe.Pointer(&dat[0]), C.int(len(dat)))
	return unsafe.Pointer(&res[0])
}

func (num *num_t) mask_bits(bits uint) {
	mask := new(big.Int).Lsh(BigInt1, bits)
	mask.Sub(mask, BigInt1)
	num.Int.And(&num.Int, mask)
}

func (a *num_t) split_exp(r1, r2 *num_t) {
	var bnc1, bnc2, bnn2, bnt1, bnt2 num_t

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
