package newec

import (
)

// New algo by peterdettman - https://github.com/sipa/secp256k1/pull/19
func (a *fe_t) inv(r *fe_t) {
	var x2, x3, x6, x9, x11, x22, x44, x88, x176, x220, x223, t1 fe_t
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
	for j:=1; j<11; j++ {
		x22.sqr(&x22)
	}
	x22.mul(&x22, &x11)

	x22.sqr(&x44)
	for j:=1; j<22; j++ {
		x44.sqr(&x44)
	}
	x44.mul(&x44, &x22)

	x44.sqr(&x88)
	for j:=1; j<44; j++ {
		x88.sqr(&x88)
	}
	x88.mul(&x88, &x44)

	x88.sqr(&x176)
	for j:=1; j<88; j++ {
		x176.sqr(&x176)
	}
	x176.mul(&x176, &x88)

	x176.sqr(&x220)
	for j:=1; j<44; j++ {
		x220.sqr(&x220)
	}
	x220.mul(&x220, &x44)

	x220.sqr(&x223)
	x223.sqr(&x223)
	x223.sqr(&x223)
	x223.mul(&x223, &x3)


	x223.sqr(&t1)
	for j:=1; j<23; j++ {
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


// New algo by peterdettman - https://github.com/sipa/secp256k1/pull/19
func (a *fe_t) sqrt(r *fe_t) {
	var x2, x3 fe_t
	a.sqr(&x2)
	x2.mul(&x2, a)

	x2.sqr(&x3)
	x3.mul(&x3, a)

	x6 := x3
	x6.sqr(&x6)
	x6.sqr(&x6)
	x6.sqr(&x6)
	x6.mul(&x6, &x3)

	x9 := x6
	x9.sqr(&x9)
	x9.sqr(&x9)
	x9.sqr(&x9)
	x9.mul(&x9, &x3)

	x11 := x9
	x11.sqr(&x11)
	x11.sqr(&x11)
	x11.mul(&x11, &x2)

	x22 := x11
	for j:=0; j<11; j++ {
		x22.sqr(&x22)
	}
	x22.mul(&x22, &x11)

	x44 := x22
	for j:=0; j<22; j++ {
		x44.sqr(&x44)
	}
	x44.mul(&x44, &x22)

	x88 := x44
	for j:=0; j<44; j++ {
		x88.sqr(&x88)
	}
	x88.mul(&x88, &x44)

	x176 := x88
	for j:=0; j<88; j++ {
		x176.sqr(&x176)
	}
	x176.mul(&x176, &x88)

	x220 := x176
	for j:=0; j<44; j++ {
		x220.sqr(&x220)
	}
	x220.mul(&x220, &x44)

	x223 := x220
	x223.sqr(&x223)
	x223.sqr(&x223)
	x223.sqr(&x223)
	x223.mul(&x223, &x3)

	t1 := x223
	for j:=0; j<23; j++ {
		t1.sqr(&t1)
	}
	t1.mul(&t1, &x22)
	for j:=0; j<6; j++ {
		t1.sqr(&t1)
	}
	t1.mul(&t1, &x2)
	t1.sqr(&t1)
	t1.sqr(r)
}


func (a *fe_t) inv_var(r *fe_t) {
	var b [32]byte
	var c fe_t
	c = *a
	c.normalize()
	c.get_b32(b[:])
	var n num_t
	n.SetBytes(b[:])
	n.mod_inv(&n, &secp256k1.p)
	r.set_bytes(n.Bytes())
}


/* Original algo by sipa:
func (a *fe_t) inv(r *fe_t) {
	// calculate a^p, with p={45,63,1019,1023}
	var x, a2, a3, a4, a5, a10, a11, a21, a42, a45, a63, a126, a252, a504, a1008, a1019, a1023 fe_t
	var i, j int
	a.sqr(&a2)
	a2.mul(&a3, a)
	a2.sqr(&a4)
	a4.mul(&a5, a)
	a5.sqr(&a10)
	a10.mul(&a11, a)
	a11.mul(&a21, &a10)
	a21.sqr(&a42)
	a42.mul(&a45, &a3)
	a42.mul(&a63, &a21)
	a63.sqr(&a126)
	a126.sqr(&a252)
	a252.sqr(&a504)
	a504.sqr(&a1008)
	a1008.mul(&a1019, &a11)
	a1019.mul(&a1023, &a4)
	x = a63
	for i=0; i<21; i++ {
		for j=0; j<10; j++ {
			x.sqr(&x)
		}
		x.mul(&x, &a1023)
	}
	for j=0; j<10; j++ {
		x.sqr(&x)
	}
	x.mul(&x, &a1019)
	for i=0; i<2; i++ {
		for j=0; j<10; j++ {
			x.sqr(&x)
		}
		x.mul(&x, &a1023)
	}
	for j=0; j<10; j++ {
		x.sqr(&x)
	}
	x.mul(r, &a45)
}
*/


/* Original algo by sipa
func (a *fe_t) sqrt(r *fe_t) {
	var x, a2, a3, a6, a12, a15, a30, a60, a120, a240, a255, a510, a750, a780, a1020, a1022, a1023 fe_t
	var i, j int
	// calculate a^p, with p={15,780,1022,1023}
	a.sqr(&a2)
	a2.mul(&a3, a)
	a3.sqr(&a6)
	a6.sqr(&a12)
	a12.mul(&a15, &a3)
	a15.sqr(&a30)
	a30.sqr(&a60)
	a60.sqr(&a120)
	a120.sqr(&a240)
	a240.mul(&a255, &a15)
	a255.sqr(&a510)
	a510.mul(&a750, &a240)
	a750.mul(&a780, &a30)
	a510.sqr(&a1020)
	a1020.mul(&a1022, &a2)
	a1022.mul(&a1023, a)
	x = a15
	for i=0; i<21; i++ {
		for j=0; j<10; j++ {
			x.sqr(&x)
		}
		x.mul(&x, &a1023)
	}
	for j=0; j<10; j++ {
		x.sqr(&x)
	}
	x.mul(&x, &a1022)
	for i=0; i<2; i++ {
		for j=0; j<10; j++ {
			x.sqr(&x)
		}
		x.mul(&x, &a1023)
	}
	for j=0; j<10; j++ {
		x.sqr(&x)
	}
	x.mul(r, &a780)
}
*/
