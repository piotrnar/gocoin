package secp256k1

import (
)


/* New algo by peterdettman - https://github.com/sipa/TheCurve/pull/19 */
func (a *Fe_t) inv(r *Fe_t) {
	var x2, x3, x6, x9, x11, x22, x44, x88, x176, x220, x223, t1 Fe_t
	var j int

	a.sqr(&x2)
	x2.mul(&x2, a)

	x2.sqr(&x3)
	x3.mul(&x3, a)

	x3.sqr(&x6)
	x6.sqr(&x6)
	x6.sqr(&x6)
	x6.mul(&x6, &x3)

	x6.sqr(&x9)
	x9.sqr(&x9)
	x9.sqr(&x9)
	x9.mul(&x9, &x3)

	x9.sqr(&x11)
	x11.sqr(&x11)
	x11.mul(&x11, &x2)

	x11.sqr(&x22)
	for j=1; j<11; j++ {
		x22.sqr(&x22)
	}
	x22.mul(&x22, &x11)

	x22.sqr(&x44)
	for j=1; j<22; j++ {
		x44.sqr(&x44)
	}
	x44.mul(&x44, &x22)

	x44.sqr(&x88)
	for j=1; j<44; j++ {
		x88.sqr(&x88)
	}
	x88.mul(&x88, &x44)

	x88.sqr(&x176)
	for j=1; j<88; j++ {
		x176.sqr(&x176)
	}
	x176.mul(&x176, &x88)

	x176.sqr(&x220)
	for j=1; j<44; j++ {
		x220.sqr(&x220)
	}
	x220.mul(&x220, &x44)

	x220.sqr(&x223)
	x223.sqr(&x223)
	x223.sqr(&x223)
	x223.mul(&x223, &x3)


	x223.sqr(&t1)
	for j=1; j<23; j++ {
		t1.sqr(&t1)
	}
	t1.mul(&t1, &x22)
	t1.sqr(&t1)
	t1.sqr(&t1)
	t1.sqr(&t1)
	t1.sqr(&t1)
	t1.sqr(&t1)
	t1.mul(&t1, a)
	t1.sqr(&t1)
	t1.sqr(&t1)
	t1.sqr(&t1)
	t1.mul(&t1, &x2)
	t1.sqr(&t1)
	t1.sqr(&t1)
	t1.mul(r, a)
}


/* New algo by peterdettman - https://github.com/sipa/TheCurve/pull/19 */
func (a *Fe_t) sqrt(r *Fe_t) {
	var x2, x3, x6, x9, x11, x22, x44, x88, x176, x220, x223, t1 Fe_t
	var j int

	a.sqr(&x2)
	x2.mul(&x2, a)

	x2.sqr(&x3)
	x3.mul(&x3, a)

	x3.sqr(&x6)
	x6.sqr(&x6)
	x6.sqr(&x6)
	x6.mul(&x6, &x3)

	x6.sqr(&x9)
	x9.sqr(&x9)
	x9.sqr(&x9)
	x9.mul(&x9, &x3)

	x9.sqr(&x11)
	x11.sqr(&x11)
	x11.mul(&x11, &x2)

	x11.sqr(&x22)
	for j=1; j<11; j++ {
		x22.sqr(&x22)
	}
	x22.mul(&x22, &x11)

	x22.sqr(&x44)
	for j=1; j<22; j++ {
		x44.sqr(&x44)
	}
	x44.mul(&x44, &x22)

	x44.sqr(&x88)
	for j=1; j<44; j++ {
		x88.sqr(&x88)
	}
	x88.mul(&x88, &x44)

	x88.sqr(&x176)
	for j=1; j<88; j++ {
		x176.sqr(&x176)
	}
	x176.mul(&x176, &x88)

	x176.sqr(&x220)
	for j=1; j<44; j++ {
		x220.sqr(&x220)
	}
	x220.mul(&x220, &x44)

	x220.sqr(&x223)
	x223.sqr(&x223)
	x223.sqr(&x223)
	x223.mul(&x223, &x3)

	x223.sqr(&t1)
	for j=1; j<23; j++ {
		t1.sqr(&t1)
	}
	t1.mul(&t1, &x22)
	for j=0; j<6; j++ {
		t1.sqr(&t1)
	}
	t1.mul(&t1, &x2)
	t1.sqr(&t1)
	t1.sqr(r)
}


func (a *Fe_t) inv_var(r *Fe_t) {
	var b [32]byte
	var c Fe_t
	c = *a
	c.Normalize()
	c.GetB32(b[:])
	var n Number
	n.SetBytes(b[:])
	n.mod_inv(&n, &TheCurve.p)
	r.SetBytes(n.Bytes())
}

