package newec

import (
	"fmt"
//	"encoding/hex"
)

type gej_t struct {
	x, y, z fe_t
	infinity bool
}

func (gej gej_t) print(lab string) {
	if gej.infinity {
		fmt.Println(lab + " - INFINITY")
		return
	}
	fmt.Println(lab + ".X", gej.x.String())
	fmt.Println(lab + ".Y", gej.y.String())
	fmt.Println(lab + ".Z", gej.z.String())
}


func (r *gej_t) set_ge(a *ge_t) {
	r.infinity = a.infinity
	r.x = a.x
	r.y = a.y
	r.z.set_int(1)
}

func (r *gej_t) is_infinity() bool {
	return r.infinity
}

func (a *gej_t) is_valid() bool {
	if a.infinity {
		return false
	}
	var y2, x3, z2, z6 fe_t
	a.y.sqr(&y2)
	a.x.sqr(&x3); x3.mul(&x3, &a.x)
	a.z.sqr(&z2)
	z2.sqr(&z6); z6.mul(&z6, &z2)
	z6.mul_int(7)
	x3.add(&z6)
	y2.normalize()
	x3.normalize()
	return y2.equal(&x3)
}

func (a *gej_t) get_x(r *fe_t) {
	var zi2 fe_t
	a.z.inv_var(&zi2)
	zi2.sqr(&zi2)
	a.x.mul(r, &zi2)
}

func (a *gej_t) normalize() {
	a.x.normalize()
	a.y.normalize()
	a.z.normalize()
}

func (a *gej_t) equal(b *gej_t) bool {
	if a.infinity != b.infinity {
		return false
	}
	// TODO: check it it does not affect performance
	a.normalize()
	b.normalize()
	return a.x.equal(&b.x) && a.y.equal(&b.y) && a.z.equal(&b.z)
}


func (a *gej_t) precomp(w int) (pre []gej_t) {
	var d gej_t
	pre = make([]gej_t, (1 << (uint(w)-2)))
	pre[0] = *a;
	pre[0].double(&d)
	for i:=1 ; i<len(pre); i++ {
		d.add(&pre[i], &pre[i-1])
	}
	return
}


func (a *gej_t) ecmult(r *gej_t, na, ng *num_t) {
	var na_1, na_lam, ng_1, ng_128 num_t

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
	var a_lam gej_t
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

	r.infinity = true

	var tmpj gej_t
	var tmpa ge_t
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


func (a *gej_t) neg(r *gej_t) {
	r.infinity = a.infinity
	r.x = a.x
	r.y = a.y
	r.z = a.z
	r.y.normalize()
	r.y.negate(&r.y, 1)
}

func (a *gej_t) mul_lambda(r *gej_t) {
	*r = *a
	r.x.mul(&r.x, &beta)
}

