package ecver

import (
	"encoding/hex"
)

// GE
type ge_t struct {
	x, y fe_t
	infinity bool
}

func (r *ge_t) set_gej(a *gej_t) {
	var z2, z3 fe_t
	a.z.inv_s()
	a.z.sqr_p(&z2)
	a.z.mul_p(&z3, &z2)
	a.x.mul_s(&z2)
	a.y.mul_s(&z3)
	a.z.Set(BigInt1)
	r.infinity = a.infinity
	r.x = a.x
	r.y = a.y
}

func (ge *ge_t) print(lab string) {
	if ge.infinity {
		println(lab + " - infinity")
		return
	}
	println(lab + ".x:", hex.EncodeToString(ge.x.Bytes()))
	println(lab + ".y:", hex.EncodeToString(ge.y.Bytes()))
}

func (a *ge_t) neg_p(r *ge_t) {
	r.infinity = a.infinity
	if !a.infinity {
		r.infinity = false
		r.x = a.x
		a.y.neg_p(&r.y)
	}
	return
}

func (a *ge_t) precomp(w int) (pre []ge_t) {
	pre = make([]ge_t, (1 << (uint(w)-2)))
	pre[0] = *a;
	var x, d, tmp gej_t
	x.set_ge(a)
	x.double_p(&d)
	for i:=1 ; i<len(pre); i++ {
		d.add_ge_p(&tmp, &pre[i-1])
		pre[i].set_gej(&tmp)
	}
	return
}
