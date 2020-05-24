package secp256k1

import (
	"fmt"
	"math/big"
	"encoding/hex"
)

func (a *Field) String() string {
	var tmp [32]byte
	b := *a
	b.Normalize()
	b.GetB32(tmp[:])
	return hex.EncodeToString(tmp[:])
}

func (a *Field) Print(lab string) {
	fmt.Println(lab + ":", a.String())
}

func (a *Field) GetBig() (r *big.Int) {
	a.Normalize()
	r = new(big.Int)
	var tmp [32]byte
	a.GetB32(tmp[:])
	r.SetBytes(tmp[:])
	return
}

func (r *Field) SetBytes(a []byte) {
	if len(a)>32 {
		panic("too many bytes to set")
	}
	if len(a)==32 {
		r.SetB32(a)
	} else {
		var buf [32]byte
		copy(buf[32-len(a):], a)
		r.SetB32(buf[:])
	}
}

func (r *Field) SetHex(s string) {
	d, _ := hex.DecodeString(s)
	r.SetBytes(d)
}

func (a *Field) IsOdd() bool {
	return (a.n[0]&1) != 0
}

/* New algo by peterdettman - https://github.com/sipa/TheCurve/pull/19 */
func (a *Field) Inv(r *Field) {
	var x2, x3, x6, x9, x11, x22, x44, x88, x176, x220, x223, t1 Field
	var j int

	a.Sqr(&x2)
	x2.Mul(&x2, a)

	x2.Sqr(&x3)
	x3.Mul(&x3, a)

	x3.Sqr(&x6)
	x6.Sqr(&x6)
	x6.Sqr(&x6)
	x6.Mul(&x6, &x3)

	x6.Sqr(&x9)
	x9.Sqr(&x9)
	x9.Sqr(&x9)
	x9.Mul(&x9, &x3)

	x9.Sqr(&x11)
	x11.Sqr(&x11)
	x11.Mul(&x11, &x2)

	x11.Sqr(&x22)
	for j=1; j<11; j++ {
		x22.Sqr(&x22)
	}
	x22.Mul(&x22, &x11)

	x22.Sqr(&x44)
	for j=1; j<22; j++ {
		x44.Sqr(&x44)
	}
	x44.Mul(&x44, &x22)

	x44.Sqr(&x88)
	for j=1; j<44; j++ {
		x88.Sqr(&x88)
	}
	x88.Mul(&x88, &x44)

	x88.Sqr(&x176)
	for j=1; j<88; j++ {
		x176.Sqr(&x176)
	}
	x176.Mul(&x176, &x88)

	x176.Sqr(&x220)
	for j=1; j<44; j++ {
		x220.Sqr(&x220)
	}
	x220.Mul(&x220, &x44)

	x220.Sqr(&x223)
	x223.Sqr(&x223)
	x223.Sqr(&x223)
	x223.Mul(&x223, &x3)


	x223.Sqr(&t1)
	for j=1; j<23; j++ {
		t1.Sqr(&t1)
	}
	t1.Mul(&t1, &x22)
	t1.Sqr(&t1)
	t1.Sqr(&t1)
	t1.Sqr(&t1)
	t1.Sqr(&t1)
	t1.Sqr(&t1)
	t1.Mul(&t1, a)
	t1.Sqr(&t1)
	t1.Sqr(&t1)
	t1.Sqr(&t1)
	t1.Mul(&t1, &x2)
	t1.Sqr(&t1)
	t1.Sqr(&t1)
	t1.Mul(r, a)
}


/* New algo by peterdettman - https://github.com/sipa/TheCurve/pull/19 */
func (a *Field) Sqrt(r *Field) {
	var x2, x3, x6, x9, x11, x22, x44, x88, x176, x220, x223, t1 Field
	var j int

	a.Sqr(&x2)
	x2.Mul(&x2, a)

	x2.Sqr(&x3)
	x3.Mul(&x3, a)

	x3.Sqr(&x6)
	x6.Sqr(&x6)
	x6.Sqr(&x6)
	x6.Mul(&x6, &x3)

	x6.Sqr(&x9)
	x9.Sqr(&x9)
	x9.Sqr(&x9)
	x9.Mul(&x9, &x3)

	x9.Sqr(&x11)
	x11.Sqr(&x11)
	x11.Mul(&x11, &x2)

	x11.Sqr(&x22)
	for j=1; j<11; j++ {
		x22.Sqr(&x22)
	}
	x22.Mul(&x22, &x11)

	x22.Sqr(&x44)
	for j=1; j<22; j++ {
		x44.Sqr(&x44)
	}
	x44.Mul(&x44, &x22)

	x44.Sqr(&x88)
	for j=1; j<44; j++ {
		x88.Sqr(&x88)
	}
	x88.Mul(&x88, &x44)

	x88.Sqr(&x176)
	for j=1; j<88; j++ {
		x176.Sqr(&x176)
	}
	x176.Mul(&x176, &x88)

	x176.Sqr(&x220)
	for j=1; j<44; j++ {
		x220.Sqr(&x220)
	}
	x220.Mul(&x220, &x44)

	x220.Sqr(&x223)
	x223.Sqr(&x223)
	x223.Sqr(&x223)
	x223.Mul(&x223, &x3)

	x223.Sqr(&t1)
	for j=1; j<23; j++ {
		t1.Sqr(&t1)
	}
	t1.Mul(&t1, &x22)
	for j=0; j<6; j++ {
		t1.Sqr(&t1)
	}
	t1.Mul(&t1, &x2)
	t1.Sqr(&t1)
	t1.Sqr(r)
}


func (a *Field) InvVar(r *Field) {
	var b [32]byte
	var c Field
	c = *a
	c.Normalize()
	c.GetB32(b[:])
	var n Number
	n.SetBytes(b[:])
	n.mod_inv(&n, &TheCurve.p)
	r.SetBytes(n.Bytes())
}
