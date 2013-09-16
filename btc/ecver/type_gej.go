package ecver

import (
//	"os"
	"math/big"
	"encoding/hex"
)


type secp256k1_gej_t struct {
    x, y, z secp256k1_fe_t
    infinity bool
}

func (gej *secp256k1_gej_t) set_ge(val *secp256k1_ge_t) {
	gej.infinity = val.infinity
	gej.x.Set(&val.x.Int)
	gej.y.Set(&val.y.Int)
	gej.z.Set(BigInt1)
}

func (gej secp256k1_gej_t) print(lab string) {
	if gej.infinity {
		println("GEJ." + lab + "- INFINITY")
		return
	}
	println("GEJ." + lab + ".x", hex.EncodeToString(gej.x.Bytes()))
	println("GEJ." + lab + ".y", hex.EncodeToString(gej.y.Bytes()))
	println("GEJ." + lab + ".z", hex.EncodeToString(gej.z.Bytes()))
}


func (a *secp256k1_gej_t) add_ge_p(rr *secp256k1_gej_t, b *secp256k1_ge_t) {
	if a.infinity {
		rr.set_ge(b)
		return;
	}
	if b.infinity {
		rr = a
		return
	}

	var z12, u2, s2, h, h2, h3, i, i2, t secp256k1_fe_t
	var r secp256k1_gej_t
	a.z.sqr_p(&z12)
	b.x.mul_p(&u2, &z12)

	b.y.mul_p(&s2, &z12)
	s2.mul_s(&a.z)

	if a.x.equal(&u2) {
		if a.y.equal(&s2) {
			a.double_p(&r)
		} else {
			r.infinity = true
		}
		return
	}

	a.x.neg_p(&h)
	h.add_s(&u2)
	a.y.neg_p(&i)
	i.add_s(&s2)
	i.sqr_p(&i2)
	h.sqr_p(&h2)
	h.mul_p(&h3, &h2)

	a.z.mul_p(&r.z, &h)
	a.x.mul_p(&t, &h2)
	t.add_p(&r.x, &t)
	r.x.add_s(&h3)
	r.x.neg_s()
	r.x.add_s(&i2)

	r.x.neg_p(&r.y)
	r.y.add_s(&t)
	r.y.mul_s(&i)
	h3.mul_s(&a.y)
	h3.neg_s()
	r.y.add_s(&h3)

	*rr = r

	return
}


func (a *secp256k1_gej_t) add_p(rr *secp256k1_gej_t, b *secp256k1_gej_t) {
	if a.infinity {
		*rr = *b
		return
	}
	if b.infinity {
		*rr = *a
		return
	}

	var z22, z12, u1, u2, s1, s2, h, h2, h3, i, i2, t secp256k1_fe_t
	var r secp256k1_gej_t

	r.infinity = false
	b.z.sqr_p(&z22)
	a.z.sqr_p(&z12)

	a.x.mul_p(&u1, &z22)
	b.x.mul_p(&u2, &z12)
	a.y.mul_p(&s1, &z22)
	s1.mul_s(&b.z)
	b.y.mul_p(&s2, &z12)
	s2.mul_s(&a.z)

	if u1.equal(&u2) {
		if s1.equal(&s2) {
			a.double_p(&r)
		} else {
			r.infinity = true
		}
		return
	}

	u1.neg_p(&h)
	h.add_s(&u2)
	s1.neg_p(&i)
	i.add_s(&s2)
	i.sqr_p(&i2)
	h.sqr_p(&h2)
	h.mul_p(&h3, &h2)

	a.z.mul_p(&r.z, &b.z)
	r.z.mul_s(&h)

	u1.mul_p(&t, &h2)

	t.add_p(&r.x, &t)
	r.x.add_s(&h3)
	r.x.neg_s()
	r.x.add_s(&i2)


	r.x.neg_p(&r.y)
	r.y.add_s(&t)
	r.y.mul_s(&i)

	h3.mul_s(&s1)
	h3.neg_s()
	r.y.add_s(&h3)

	*rr = r

	return
}


func (a *secp256k1_gej_t) mul_lambda_s() {
	a.x.mul_s(&beta)
	return
}


func (a *secp256k1_gej_t) neg_p(rr *secp256k1_gej_t) {
	var r secp256k1_gej_t
	r.infinity = a.infinity
	r.x = a.x
	a.y.neg_p(&r.y)
	r.z = a.z
	*rr = r
	return
}


func (a *secp256k1_gej_t) equal(b *secp256k1_gej_t) bool {
	if a.infinity != b.infinity {
		return false
	}
	if !a.x.equal(&b.x) {
		return false
	}
	if !a.y.equal(&b.y) {
		return false
	}
	return a.z.equal(&b.z)
}


func (a *secp256k1_gej_t) precomp(w int) (pre []secp256k1_gej_t) {
	var d secp256k1_gej_t
	pre = make([]secp256k1_gej_t, (1 << (uint(w)-2)))
	pre[0] = *a;
	pre[0].double_p(&d)
	for i:=1 ; i<len(pre); i++ {
		d.add_p(&pre[i], &pre[i-1])
	}
	return
}


func (a *secp256k1_gej_t) get_x_p(r *secp256k1_fe_t) {
	var zi2 secp256k1_fe_t
	a.z.inv_p(&zi2)
	zi2.sqr_s()
	a.x.mul_p(r, &zi2)
	return
}


func (in *secp256k1_gej_t) double_p(rr *secp256k1_gej_t) {
	if in.infinity || in.y.Sign()==0 {
		rr.infinity = true
		return
	}

	var r secp256k1_gej_t
	a := new(big.Int).Mul(&in.x.Int, &in.x.Int) //X12
	b := new(big.Int).Mul(&in.y.Int, &in.y.Int) //Y12
	c := new(big.Int).Mul(b, b) //B2

	d := new(big.Int).Add(&in.x.Int, b) //X1+B
	d.Mul(d, d)                 //(X1+B)2
	d.Sub(d, a)                 //(X1+B)2-A
	d.Sub(d, c)                 //(X1+B)2-A-C
	d.Mul(d, big.NewInt(2))     //2*((X1+B)2-A-C)

	e := new(big.Int).Mul(big.NewInt(3), a) //3*A
	f := new(big.Int).Mul(e, e)             //E2

	x3 := new(big.Int).Mul(big.NewInt(2), d) //2*D
	x3.Sub(f, x3)                            //F-2*D
	x3.Mod(x3, secp256k1.P)

	y3 := new(big.Int).Sub(d, x3)                  //D-X3
	y3.Mul(e, y3)                                  //E*(D-X3)
	y3.Sub(y3, new(big.Int).Mul(big.NewInt(8), c)) //E*(D-X3)-8*C
	y3.Mod(y3, secp256k1.P)

	z3 := new(big.Int).Mul(&in.y.Int, &in.z.Int) //Y1*Z1
	z3.Mul(big.NewInt(2), z3)    //3*Y1*Z1
	z3.Mod(z3, secp256k1.P)

	r.x.Int.Set(x3)
	r.y.Int.Set(y3)
	r.z.Int.Set(z3)

	*rr = r

	return
}


/*
func (a *secp256k1_gej_t) ___double() (r *secp256k1_gej_t) {
	t5 := &a.y

	r = new(secp256k1_gej_t)
	if a.infinity || t5.Sign()==0 {
		r.infinity = true
		return
	}

	r.z = *t5.mul(&a.z)
	r.z.mul_int(2)

	t1 := a.x.sqr()
	t1.mul_int(3)

	t2 := t1.sqr()

	t3 := t5.sqr()
	t3.mul_int(2)

	t4 := t3.sqr()
	t4.mul_int(2)

	t3 = a.x.mul(t3)
	r.x.Set(&t3.Int)
	r.x.mul_int(4)

	r.x = *r.x.neg().add(t2)

	t2 = t2.neg()

	t3.mul_int(6)
	t3 = t3.add(t2)

	r.y = *t1.mul(t3)
	t2 = t4.neg()
	r.y = *r.y.add(t2)
	return
}
*/
