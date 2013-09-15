package ecver

import (
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

func (a *secp256k1_gej_t) add_ge(b *secp256k1_ge_t) (r *secp256k1_gej_t) {
	r = new(secp256k1_gej_t)
	if a.infinity {
		r.set_ge(b)
		return;
	}
	if b.infinity {
		r = a
		return
	}

	r.infinity = false
	z12 := a.z.sqr()
	u1 := &a.x
	u2 := b.x.mul(z12)
	s1 := &a.y
	s2 := b.y.mul(z12).mul(&a.z)

	if u1.equal(u2) {
		if s1.equal(s2) {
			r = a.double()
		} else {
			r.infinity = true
		}
		return
	}

	h := u1.neg().add(u2)
	i := s1.neg().add(s2)
	i2 := i.sqr()
	h2 := h.sqr()
	h3 := h.mul(h2)
	r.z = *a.z.mul(h)
	t := u1.mul(h2)
	r.x = *t.add(t).add(h3).neg().add(i2)
	r.y = *r.x.neg().add(t).mul(i).add(h3.mul(s1).neg())

	return
}


func (a *secp256k1_gej_t) add(b *secp256k1_gej_t) (r *secp256k1_gej_t) {
	r = new(secp256k1_gej_t)
	if a.infinity {
		*r = *b
		return
	}
	if b.infinity {
		*r = *a
		return
	}

	r.infinity = false
	z22 := b.z.sqr()
	z12 := a.z.sqr()

	u1 := a.x.mul(z22)
	u2 := b.x.mul(z12)
	s1 := a.y.mul(z22).mul(&b.z)
	s2 := b.y.mul(z12).mul(&a.z)

	if u1.equal(u2) {
		if s1.equal(s2) {
			r = a.double()
		} else {
			r.infinity = true
		}
		return
	}

	h := u1.neg().add(u2)
	i := s1.neg().add(s2)
	i2 := i.sqr()
	h2 := h.sqr()
	h3 := h.mul(h2)
	r.z = *a.z.mul(&b.z).mul(h)
	t := u1.mul(h2)
	r.x = *t.add(t).add(h3).neg().add(i2)

	r.y = *r.x.neg().add(t).mul(i)
	r.y = *r.y.add(h3.mul(s1).neg())

	return
}


func (a *secp256k1_gej_t) mul_lambda() (r *secp256k1_gej_t) {
	r = new(secp256k1_gej_t)
	*r = *a;
	r.x = *r.x.mul(&beta)
	return
}


func (a *secp256k1_gej_t) neg() (r *secp256k1_gej_t) {
	r = new(secp256k1_gej_t)
	r.infinity = a.infinity
	r.x = a.x
	r.y = *a.y.neg()
	r.z = a.z
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
	pre = make([]secp256k1_gej_t, (1 << (uint(w)-2)))
	pre[0] = *a;
	d := pre[0].double()
	for i:=1 ; i<len(pre); i++ {
		pre[i] = *d.add(&pre[i-1])
	}
	return
}


func (a *secp256k1_gej_t) get_x() (r *secp256k1_fe_t) {
	zi2 := a.z.inv().sqr()
	r = a.x.mul(zi2)
	return
}


func (in *secp256k1_gej_t) double() (r *secp256k1_gej_t) {
	r = new(secp256k1_gej_t)
	if in.infinity || in.y.Sign()==0 {
		r.infinity = true
		return
	}

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

	return
}



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
