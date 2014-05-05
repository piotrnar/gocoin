package secp256k1

import (
	"fmt"
//	"encoding/hex"
)


type XYZ_t struct {
	X, Y, Z Fe_t
	Infinity bool
}

func (gej XYZ_t) Print(lab string) {
	if gej.Infinity {
		fmt.Println(lab + " - INFINITY")
		return
	}
	fmt.Println(lab + ".X", gej.X.String())
	fmt.Println(lab + ".Y", gej.Y.String())
	fmt.Println(lab + ".Z", gej.Z.String())
}


func (r *XYZ_t) set_ge(a *XY) {
	r.Infinity = a.Infinity
	r.X = a.X
	r.Y = a.Y
	r.Z.SetInt(1)
}

func (r *XYZ_t) is_infinity() bool {
	return r.Infinity
}

func (a *XYZ_t) IsValid() bool {
	if a.Infinity {
		return false
	}
	var y2, x3, z2, z6 Fe_t
	a.Y.sqr(&y2)
	a.X.sqr(&x3); x3.mul(&x3, &a.X)
	a.Z.sqr(&z2)
	z2.sqr(&z6); z6.mul(&z6, &z2)
	z6.MulInt(7)
	x3.set_add(&z6)
	y2.Normalize()
	x3.Normalize()
	return y2.Equals(&x3)
}

func (a *XYZ_t) get_x(r *Fe_t) {
	var zi2 Fe_t
	a.Z.inv_var(&zi2)
	zi2.sqr(&zi2)
	a.X.mul(r, &zi2)
}

func (a *XYZ_t) Normalize() {
	a.X.Normalize()
	a.Y.Normalize()
	a.Z.Normalize()
}

func (a *XYZ_t) Equals(b *XYZ_t) bool {
	if a.Infinity != b.Infinity {
		return false
	}
	// TODO: check it it does not affect performance
	a.Normalize()
	b.Normalize()
	return a.X.Equals(&b.X) && a.Y.Equals(&b.Y) && a.Z.Equals(&b.Z)
}


func (a *XYZ_t) precomp(w int) (pre []XYZ_t) {
	var d XYZ_t
	pre = make([]XYZ_t, (1 << (uint(w)-2)))
	pre[0] = *a;
	pre[0].double(&d)
	for i:=1 ; i<len(pre); i++ {
		d.add(&pre[i], &pre[i-1])
	}
	return
}


func (a *XYZ_t) ecmult(r *XYZ_t, na, ng *Number) {
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
	var a_lam XYZ_t
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

	var tmpj XYZ_t
	var tmpa XY
	var n int

	for i:=bits-1; i>=0; i-- {
		r.double(r)

		if i < bits_na_1 {
			n = wnaf_na_1[i]
			if n > 0 {
				r.add(r, &pre_a_1[((n)-1)/2])
			} else if n != 0 {
				pre_a_1[(-(n)-1)/2].neg(&tmpj)
				r.add(r, &tmpj)
			}
		}

		if i < bits_na_lam {
			n = wnaf_na_lam[i]
			if n > 0 {
				r.add(r, &pre_a_lam[((n)-1)/2])
			} else if n != 0 {
				pre_a_lam[(-(n)-1)/2].neg(&tmpj)
				r.add(r, &tmpj)
			}
		}

		if i < bits_ng_1 {
			n = wnaf_ng_1[i]
			if n > 0 {
				r.add_ge(r, &pre_g[((n)-1)/2])
			} else if n != 0 {
				pre_g[(-(n)-1)/2].neg(&tmpa)
				r.add_ge(r, &tmpa)
			}
		}

		if i < bits_ng_128 {
			n = wnaf_ng_128[i]
			if n > 0 {
				r.add_ge(r, &pre_g_128[((n)-1)/2])
			} else if n != 0 {
				pre_g_128[(-(n)-1)/2].neg(&tmpa)
				r.add_ge(r, &tmpa)
			}
		}
	}
}


func (a *XYZ_t) neg(r *XYZ_t) {
	r.Infinity = a.Infinity
	r.X = a.X
	r.Y = a.Y
	r.Z = a.Z
	r.Y.Normalize()
	r.Y.Negate(&r.Y, 1)
}

func (a *XYZ_t) mul_lambda(r *XYZ_t) {
	*r = *a
	r.X.mul(&r.X, &TheCurve.beta)
}


func (a *XYZ_t) double(r *XYZ_t) {
	var t1, t2, t3, t4, t5 Fe_t

	t5 = a.Y
	t5.Normalize()
	if (a.Infinity || t5.IsZero()) {
		r.Infinity = true
		return
	}

	t5.mul(&r.Z, &a.Z)
	r.Z.MulInt(2)
	a.X.sqr(&t1)
	t1.MulInt(3)
	t1.sqr(&t2)
	t5.sqr(&t3)
	t3.MulInt(2)
	t3.sqr(&t4)
	t4.MulInt(2)
	a.X.mul(&t3, &t3)
	r.X = t3
	r.X.MulInt(4)
	r.X.Negate(&r.X, 4)
	r.X.set_add(&t2)
	t2.Negate(&t2, 1)
	t3.MulInt(6)
	t3.set_add(&t2)
	t1.mul(&r.Y, &t3)
	t4.Negate(&t2, 2)
	r.Y.set_add(&t2)
	r.Infinity = false
}


func (a *XYZ_t) add_ge(r *XYZ_t, b *XY) {
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
	var z12, u1, u2, s1, s2 Fe_t
	a.Z.sqr(&z12)
	u1 = a.X
	u1.Normalize()
	b.X.mul(&u2, &z12)
	s1 = a.Y
	s1.Normalize()
	b.Y.mul(&s2, &z12)
	s2.mul(&s2, &a.Z)
	u1.Normalize()
	u2.Normalize()

	if u1.Equals(&u2) {
		s1.Normalize()
		s2.Normalize()
		if (s1.Equals(&s2)) {
			a.double(r)
		} else {
			r.Infinity = true
		}
		return
	}

	var h, i, i2, h2, h3, t Fe_t
	u1.Negate(&h, 1)
	h.set_add(&u2)
	s1.Negate(&i, 1)
	i.set_add(&s2)
	i.sqr(&i2)
	h.sqr(&h2)
	h.mul(&h3, &h2)
	r.Z = a.Z
	r.Z.mul(&r.Z, &h)
	u1.mul(&t, &h2)
	r.X = t
	r.X.MulInt(2)
	r.X.set_add(&h3)
	r.X.Negate(&r.X, 3)
	r.X.set_add(&i2)
	r.X.Negate(&r.Y, 5)
	r.Y.set_add(&t)
	r.Y.mul(&r.Y, &i)
	h3.mul(&h3, &s1)
	h3.Negate(&h3, 1)
	r.Y.set_add(&h3)
}


func (a *XYZ_t) add(r, b *XYZ_t) {
	if a.Infinity {
		*r = *b
		return
	}
	if b.Infinity {
		*r = *a
		return
	}
	r.Infinity = false
	var z22, z12, u1, u2, s1, s2 Fe_t

	b.Z.sqr(&z22)
	a.Z.sqr(&z12)
	a.X.mul(&u1, &z22)
	b.X.mul(&u2, &z12)
	a.Y.mul(&s1, &z22)
	s1.mul(&s1, &b.Z)
	b.Y.mul(&s2, &z12)
	s2.mul(&s2, &a.Z)
	u1.Normalize()
	u2.Normalize()
	if u1.Equals(&u2) {
		s1.Normalize()
		s2.Normalize()
		if s1.Equals(&s2) {
			a.double(r)
		} else {
			r.Infinity = true
		}
		return
	}
	var h, i, i2, h2, h3, t Fe_t

	u1.Negate(&h, 1)
	h.set_add(&u2)
	s1.Negate(&i, 1)
	i.set_add(&s2)
	i.sqr(&i2)
	h.sqr(&h2)
	h.mul(&h3, &h2)
	a.Z.mul(&r.Z, &b.Z)
	r.Z.mul(&r.Z, &h)
	u1.mul(&t, &h2)
	r.X = t
	r.X.MulInt(2)
	r.X.set_add(&h3)
	r.X.Negate(&r.X, 3)
	r.X.set_add(&i2)
	r.X.Negate(&r.Y, 5)
	r.Y.set_add(&t)
	r.Y.mul(&r.Y, &i)
	h3.mul(&h3, &s1)
	h3.Negate(&h3, 1)
	r.Y.set_add(&h3)
}
