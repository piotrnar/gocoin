package secp256k1

import (
	"fmt"
)


type XY_t struct {
	x, y Fe_t
	Infinity bool
}

func (ge *XY_t) Print(lab string) {
	if ge.Infinity {
		fmt.Println(lab + " - Infinity")
		return
	}
	fmt.Println(lab + ".x:", ge.x.String())
	fmt.Println(lab + ".y:", ge.y.String())
}

func (elem *XY_t) pubkey_parse(pub []byte) bool {
	if len(pub) == 33 && (pub[0] == 0x02 || pub[0] == 0x03) {
		elem.x.SetB32(pub[1:33])
		elem.set_xo(&elem.x, pub[0]==0x03)
	} else if len(pub) == 65 && (pub[0] == 0x04 || pub[0] == 0x06 || pub[0] == 0x07) {
		elem.x.SetB32(pub[1:33])
		elem.y.SetB32(pub[33:65])
		if (pub[0] == 0x06 || pub[0] == 0x07) && elem.y.IsOdd() != (pub[0] == 0x07) {
			return false
		}
	} else {
		return false
	}
	return elem.is_valid()
}


func (r *XY_t) set_xy(x, y *Fe_t) {
	r.Infinity = false
	r.x = *x
	r.y = *y
}


func (a *XY_t) is_valid() bool {
	if a.Infinity {
		return false
	}
	var y2, x3, c Fe_t
	a.y.sqr(&y2)
	a.x.sqr(&x3); x3.mul(&x3, &a.x)
	c.SetInt(7)
	x3.set_add(&c)
	y2.Normalize()
	x3.Normalize()
	return y2.Equals(&x3)
}


func (r *XY_t) set_gej(a *XYZ_t) {
	var z2, z3 Fe_t;
	a.z.inv_var(&a.z)
	a.z.sqr(&z2)
	a.z.mul(&z3, &z2)
	a.x.mul(&a.x, &z2)
	a.y.mul(&a.y, &z3)
	a.z.SetInt(1)
	r.Infinity = a.Infinity
	r.x = a.x
	r.y = a.y
}

func (a *XY_t) precomp(w int) (pre []XY_t) {
	pre = make([]XY_t, (1 << (uint(w)-2)))
	pre[0] = *a;
	var x, d, tmp XYZ_t
	x.set_ge(a)
	x.double(&d)
	for i:=1 ; i<len(pre); i++ {
		d.add_ge(&tmp, &pre[i-1])
		pre[i].set_gej(&tmp)
	}
	return
}

func (a *XY_t) neg(r *XY_t) {
	r.Infinity = a.Infinity
	r.x = a.x
	r.y = a.y
	r.y.Normalize()
	r.y.Negate(&r.y, 1)
}


func (r *XY_t) set_xo(x *Fe_t, odd bool) {
	var c, x2, x3 Fe_t
	r.x = *x
	x.sqr(&x2)
	x.mul(&x3, &x2)
	r.Infinity = false
	c.SetInt(7)
	c.set_add(&x3)
	c.sqrt(&r.y)
	r.y.Normalize()
	if r.y.IsOdd() != odd {
		r.y.Negate(&r.y, 1)
	}
}


// TODO: think about optimizing this one
func (pk *XY_t) multi(k, out []byte) bool {
	var B, r XYZ_t

	B.set_ge(pk)
	r = B

	seen := false
	for _, byte := range k {
		for bitNum := 0; bitNum < 8; bitNum++ {
			if seen {
				r.double(&r)
			}
			if byte&0x80 == 0x80 {
				if !seen {
					seen = true
				} else {
					r.add(&r, &B)
				}
			}
			byte <<= 1
		}
	}

	if !seen {
		return false
	}

	pk.set_gej(&r)
	pk.x.GetB32(out[1:33])
	if len(out)==65 {
		out[0] = 0x05
		pk.x.GetB32(out[33:65])
	} else {
		if pk.y.IsOdd() {
			out[0] = 0x03
		} else {
			out[0] = 0x02
		}
	}
	return true
}
