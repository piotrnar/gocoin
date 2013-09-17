package ecver

import (
	"encoding/hex"
)


type gej_t struct {
    x, y, z fe_t
    infinity bool
}

func (gej *gej_t) set_ge(val *ge_t) {
	gej.infinity = val.infinity
	gej.x.Set(&val.x.Int)
	gej.y.Set(&val.y.Int)
	gej.z.Set(BigInt1)
}

func (gej gej_t) print(lab string) {
	if gej.infinity {
		println("GEJ." + lab + "- INFINITY")
		return
	}
	println("GEJ." + lab + ".x", hex.EncodeToString(gej.x.Bytes()))
	println("GEJ." + lab + ".y", hex.EncodeToString(gej.y.Bytes()))
	println("GEJ." + lab + ".z", hex.EncodeToString(gej.z.Bytes()))
}


func (a *gej_t) add_ge_p(rr *gej_t, b *ge_t) {
	if a.infinity {
		rr.set_ge(b)
		return;
	}
	if b.infinity {
		rr = a
		return
	}

	var z12, u2, s2, h, h2, h3, i, i2, t fe_t
	var r gej_t
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


func (a *gej_t) add_p(rr *gej_t, b *gej_t) {
	if a.infinity {
		*rr = *b
		return
	}
	if b.infinity {
		*rr = *a
		return
	}

	var z22, z12, u1, u2, s1, s2, h, h2, h3, i, i2, t fe_t
	var r gej_t

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


func (a *gej_t) mul_lambda_s() {
	a.x.mul_s(&beta)
	return
}


func (a *gej_t) neg_p(rr *gej_t) {
	var r gej_t
	r.infinity = a.infinity
	r.x = a.x
	a.y.neg_p(&r.y)
	r.z = a.z
	*rr = r
	return
}


func (a *gej_t) equal(b *gej_t) bool {
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


func (a *gej_t) precomp(w int) (pre []gej_t) {
	var d gej_t
	pre = make([]gej_t, (1 << (uint(w)-2)))
	pre[0] = *a;
	pre[0].double_p(&d)
	for i:=1 ; i<len(pre); i++ {
		d.add_p(&pre[i], &pre[i-1])
	}
	return
}


func (a *gej_t) get_x_p(r *fe_t) {
	var zi2 fe_t
	a.z.inv_p(&zi2)
	zi2.sqr_s()
	a.x.mul_p(r, &zi2)
	return
}


func (a *gej_t) double_p(rr *gej_t) {
	var t1, t2, t3, t4, t5 fe_t
	var r gej_t

	t5 = a.y

	if a.infinity || t5.Sign()==0 {
		rr.infinity = true
		return
	}

	t5.mul_p(&r.z, &a.z)
	r.z.mul_int(2)

	a.x.sqr_p(&t1)
	t1.mul_int(3)

	t1.sqr_p(&t2)

	t5.sqr_p(&t3)
	t3.mul_int(2)

	t3.sqr_p(&t4)
	t4.mul_int(2)

	a.x.mul_p(&t3, &t3)
	r.x.Set(&t3.Int)
	r.x.mul_int(4)

	r.x.neg_s()
	r.x.add_s(&t2)

	t2.neg_s()

	t3.mul_int(6)
	t3.add_s(&t2)

	t1.mul_p(&r.y, &t3)
	t4.neg_p(&t2)
	r.y.add_s(&t2)

	*rr = r
	return
}
