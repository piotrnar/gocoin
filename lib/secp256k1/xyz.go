package secp256k1

import (
	"fmt"
//	"encoding/hex"
)


type XYZ struct {
	X, Y, Z Field
	Infinity bool
}

func (gej XYZ) Print(lab string) {
	if gej.Infinity {
		fmt.Println(lab + " - INFINITY")
		return
	}
	fmt.Println(lab + ".X", gej.X.String())
	fmt.Println(lab + ".Y", gej.Y.String())
	fmt.Println(lab + ".Z", gej.Z.String())
}


func (r *XYZ) SetXY(a *XY) {
	r.Infinity = a.Infinity
	r.X = a.X
	r.Y = a.Y
	r.Z.SetInt(1)
}

func (r *XYZ) IsInfinity() bool {
	return r.Infinity
}

func (a *XYZ) IsValid() bool {
	if a.Infinity {
		return false
	}
	var y2, x3, z2, z6 Field
	a.Y.Sqr(&y2)
	a.X.Sqr(&x3); x3.Mul(&x3, &a.X)
	a.Z.Sqr(&z2)
	z2.Sqr(&z6); z6.Mul(&z6, &z2)
	z6.MulInt(7)
	x3.SetAdd(&z6)
	y2.Normalize()
	x3.Normalize()
	return y2.Equals(&x3)
}

func (a *XYZ) get_x(r *Field) {
	var zi2 Field
	a.Z.InvVar(&zi2)
	zi2.Sqr(&zi2)
	a.X.Mul(r, &zi2)
}

func (a *XYZ) Normalize() {
	a.X.Normalize()
	a.Y.Normalize()
	a.Z.Normalize()
}

func (a *XYZ) Equals(b *XYZ) bool {
	if a.Infinity != b.Infinity {
		return false
	}
	// TODO: is the normalize really needed here?
	a.Normalize()
	b.Normalize()
	return a.X.Equals(&b.X) && a.Y.Equals(&b.Y) && a.Z.Equals(&b.Z)
}


func (a *XYZ) precomp(w int) (pre []XYZ) {
	var d XYZ
	pre = make([]XYZ, (1 << (uint(w)-2)))
	pre[0] = *a;
	pre[0].Double(&d)
	for i:=1 ; i<len(pre); i++ {
		d.Add(&pre[i], &pre[i-1])
	}
	return
}


func ecmult_wnaf(wnaf []int, a *Number, w uint) (ret int) {
	var zeroes uint
	var X Number
	X.Set(&a.Int)

	for X.Sign()!=0 {
		for X.Bit(0)==0 {
			zeroes++
			X.rsh(1)
		}
		word := X.rsh_x(w)
		for zeroes > 0 {
			wnaf[ret] = 0
			ret++
			zeroes--
		}
		if (word & (1 << (w-1))) != 0 {
			X.inc()
			wnaf[ret] = (word - (1 << w))
		} else {
			wnaf[ret] = word
		}
		zeroes = w-1
		ret++
	}
	return
}

// r = na*a + ng*G
func (a *XYZ) ECmult(r *XYZ, na, ng *Number) {
	var na_1, na_lam, ng_1, ng_128 Number

	// split na into na_1 and na_lam (where na = na_1 + na_lam*lambda, and na_1 and na_lam are ~128 bit)
	na.split_exp(&na_1, &na_lam)

	// split ng into ng_1 and ng_128 (where gn = gn_1 + gn_128*2^128, and gn_1 and gn_128 are ~128 bit)
	ng.split(&ng_1, &ng_128, 128)

	// build wnaf representation for na_1, na_lam, ng_1, ng_128
	var wnaf_na_1, wnaf_na_lam, wnaf_ng_1, wnaf_ng_128 [129]int
	bits_na_1 := ecmult_wnaf(wnaf_na_1[:], &na_1, WINDOW_A)
	bits_na_lam := ecmult_wnaf(wnaf_na_lam[:], &na_lam, WINDOW_A)
	bits_ng_1 := ecmult_wnaf(wnaf_ng_1[:], &ng_1, WINDOW_G)
	bits_ng_128 := ecmult_wnaf(wnaf_ng_128[:], &ng_128, WINDOW_G)

	// calculate a_lam = a*lambda
	var a_lam XYZ
	a.mul_lambda(&a_lam)

	// calculate odd multiples of a and a_lam
	pre_a_1 := a.precomp(WINDOW_A)
	pre_a_lam := a_lam.precomp(WINDOW_A)

	bits := bits_na_1
	if bits_na_lam > bits {
		bits = bits_na_lam
	}
	if bits_ng_1 > bits {
		bits = bits_ng_1
	}
	if bits_ng_128 > bits {
		bits = bits_ng_128
	}

	r.Infinity = true

	var tmpj XYZ
	var tmpa XY
	var n int

	for i:=bits-1; i>=0; i-- {
		r.Double(r)

		if i < bits_na_1 {
			n = wnaf_na_1[i]
			if n > 0 {
				r.Add(r, &pre_a_1[((n)-1)/2])
			} else if n != 0 {
				pre_a_1[(-(n)-1)/2].Neg(&tmpj)
				r.Add(r, &tmpj)
			}
		}

		if i < bits_na_lam {
			n = wnaf_na_lam[i]
			if n > 0 {
				r.Add(r, &pre_a_lam[((n)-1)/2])
			} else if n != 0 {
				pre_a_lam[(-(n)-1)/2].Neg(&tmpj)
				r.Add(r, &tmpj)
			}
		}

		if i < bits_ng_1 {
			n = wnaf_ng_1[i]
			if n > 0 {
				r.AddXY(r, &pre_g[((n)-1)/2])
			} else if n != 0 {
				pre_g[(-(n)-1)/2].Neg(&tmpa)
				r.AddXY(r, &tmpa)
			}
		}

		if i < bits_ng_128 {
			n = wnaf_ng_128[i]
			if n > 0 {
				r.AddXY(r, &pre_g_128[((n)-1)/2])
			} else if n != 0 {
				pre_g_128[(-(n)-1)/2].Neg(&tmpa)
				r.AddXY(r, &tmpa)
			}
		}
	}
}


func (a *XYZ) Neg(r *XYZ) {
	r.Infinity = a.Infinity
	r.X = a.X
	r.Y = a.Y
	r.Z = a.Z
	r.Y.Normalize()
	r.Y.Negate(&r.Y, 1)
}

func (a *XYZ) mul_lambda(r *XYZ) {
	*r = *a
	r.X.Mul(&r.X, &TheCurve.beta)
}


func (a *XYZ) Double(r *XYZ) {
	var t1, t2, t3, t4, t5 Field

	t5 = a.Y
	t5.Normalize()
	if (a.Infinity || t5.IsZero()) {
		r.Infinity = true
		return
	}

	t5.Mul(&r.Z, &a.Z)
	r.Z.MulInt(2)
	a.X.Sqr(&t1)
	t1.MulInt(3)
	t1.Sqr(&t2)
	t5.Sqr(&t3)
	t3.MulInt(2)
	t3.Sqr(&t4)
	t4.MulInt(2)
	a.X.Mul(&t3, &t3)
	r.X = t3
	r.X.MulInt(4)
	r.X.Negate(&r.X, 4)
	r.X.SetAdd(&t2)
	t2.Negate(&t2, 1)
	t3.MulInt(6)
	t3.SetAdd(&t2)
	t1.Mul(&r.Y, &t3)
	t4.Negate(&t2, 2)
	r.Y.SetAdd(&t2)
	r.Infinity = false
}


func (a *XYZ) AddXY(r *XYZ, b *XY) {
	if a.Infinity {
		r.Infinity = b.Infinity
		r.X = b.X
		r.Y = b.Y
		r.Z.SetInt(1)
		return
	}
	if b.Infinity {
		*r = *a
		return
	}
	r.Infinity = false
	var z12, u1, u2, s1, s2 Field
	a.Z.Sqr(&z12)
	u1 = a.X
	u1.Normalize()
	b.X.Mul(&u2, &z12)
	s1 = a.Y
	s1.Normalize()
	b.Y.Mul(&s2, &z12)
	s2.Mul(&s2, &a.Z)
	u1.Normalize()
	u2.Normalize()

	if u1.Equals(&u2) {
		s1.Normalize()
		s2.Normalize()
		if (s1.Equals(&s2)) {
			a.Double(r)
		} else {
			r.Infinity = true
		}
		return
	}

	var h, i, i2, h2, h3, t Field
	u1.Negate(&h, 1)
	h.SetAdd(&u2)
	s1.Negate(&i, 1)
	i.SetAdd(&s2)
	i.Sqr(&i2)
	h.Sqr(&h2)
	h.Mul(&h3, &h2)
	r.Z = a.Z
	r.Z.Mul(&r.Z, &h)
	u1.Mul(&t, &h2)
	r.X = t
	r.X.MulInt(2)
	r.X.SetAdd(&h3)
	r.X.Negate(&r.X, 3)
	r.X.SetAdd(&i2)
	r.X.Negate(&r.Y, 5)
	r.Y.SetAdd(&t)
	r.Y.Mul(&r.Y, &i)
	h3.Mul(&h3, &s1)
	h3.Negate(&h3, 1)
	r.Y.SetAdd(&h3)
}


func (a *XYZ) Add(r, b *XYZ) {
	if a.Infinity {
		*r = *b
		return
	}
	if b.Infinity {
		*r = *a
		return
	}
	r.Infinity = false
	var z22, z12, u1, u2, s1, s2 Field

	b.Z.Sqr(&z22)
	a.Z.Sqr(&z12)
	a.X.Mul(&u1, &z22)
	b.X.Mul(&u2, &z12)
	a.Y.Mul(&s1, &z22)
	s1.Mul(&s1, &b.Z)
	b.Y.Mul(&s2, &z12)
	s2.Mul(&s2, &a.Z)
	u1.Normalize()
	u2.Normalize()
	if u1.Equals(&u2) {
		s1.Normalize()
		s2.Normalize()
		if s1.Equals(&s2) {
			a.Double(r)
		} else {
			r.Infinity = true
		}
		return
	}
	var h, i, i2, h2, h3, t Field

	u1.Negate(&h, 1)
	h.SetAdd(&u2)
	s1.Negate(&i, 1)
	i.SetAdd(&s2)
	i.Sqr(&i2)
	h.Sqr(&h2)
	h.Mul(&h3, &h2)
	a.Z.Mul(&r.Z, &b.Z)
	r.Z.Mul(&r.Z, &h)
	u1.Mul(&t, &h2)
	r.X = t
	r.X.MulInt(2)
	r.X.SetAdd(&h3)
	r.X.Negate(&r.X, 3)
	r.X.SetAdd(&i2)
	r.X.Negate(&r.Y, 5)
	r.Y.SetAdd(&t)
	r.Y.Mul(&r.Y, &i)
	h3.Mul(&h3, &s1)
	h3.Negate(&h3, 1)
	r.Y.SetAdd(&h3)
}


// r = a*G
func ECmultGen(r *XYZ, a *Number) {
	var n Number;
	n.Set(&a.Int)
	r.SetXY(&prec[0][n.rsh_x(4)])
	for j:=1; j<64; j++ {
		r.AddXY(r, &prec[j][n.rsh_x(4)])
	}
	r.AddXY(r, &fin)
}
