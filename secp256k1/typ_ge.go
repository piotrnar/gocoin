package secp256k1

import (
	"fmt"
)


type XY struct {
	X, Y Fe_t
	Infinity bool
}

func (ge *XY) Print(lab string) {
	if ge.Infinity {
		fmt.Println(lab + " - Infinity")
		return
	}
	fmt.Println(lab + ".X:", ge.X.String())
	fmt.Println(lab + ".Y:", ge.Y.String())
}

func (elem *XY) ParsePubkey(pub []byte) bool {
	if len(pub) == 33 && (pub[0] == 0x02 || pub[0] == 0x03) {
		elem.X.SetB32(pub[1:33])
		elem.set_xo(&elem.X, pub[0]==0x03)
	} else if len(pub) == 65 && (pub[0] == 0x04 || pub[0] == 0x06 || pub[0] == 0x07) {
		elem.X.SetB32(pub[1:33])
		elem.Y.SetB32(pub[33:65])
		if (pub[0] == 0x06 || pub[0] == 0x07) && elem.Y.IsOdd() != (pub[0] == 0x07) {
			return false
		}
	} else {
		return false
	}
	return true
}


func (r *XY) SetXY(X, Y *Fe_t) {
	r.Infinity = false
	r.X = *X
	r.Y = *Y
}


func (a *XY) IsValid() bool {
	if a.Infinity {
		return false
	}
	var y2, x3, c Fe_t
	a.Y.sqr(&y2)
	a.X.sqr(&x3); x3.mul(&x3, &a.X)
	c.SetInt(7)
	x3.set_add(&c)
	y2.Normalize()
	x3.Normalize()
	return y2.Equals(&x3)
}


func (r *XY) set_gej(a *XYZ_t) {
	var z2, z3 Fe_t;
	a.Z.inv_var(&a.Z)
	a.Z.sqr(&z2)
	a.Z.mul(&z3, &z2)
	a.X.mul(&a.X, &z2)
	a.Y.mul(&a.Y, &z3)
	a.Z.SetInt(1)
	r.Infinity = a.Infinity
	r.X = a.X
	r.Y = a.Y
}

func (a *XY) precomp(w int) (pre []XY) {
	pre = make([]XY, (1 << (uint(w)-2)))
	pre[0] = *a;
	var X, d, tmp XYZ_t
	X.set_ge(a)
	X.double(&d)
	for i:=1 ; i<len(pre); i++ {
		d.add_ge(&tmp, &pre[i-1])
		pre[i].set_gej(&tmp)
	}
	return
}

func (a *XY) neg(r *XY) {
	r.Infinity = a.Infinity
	r.X = a.X
	r.Y = a.Y
	r.Y.Normalize()
	r.Y.Negate(&r.Y, 1)
}


func (r *XY) set_xo(X *Fe_t, odd bool) {
	var c, x2, x3 Fe_t
	r.X = *X
	X.sqr(&x2)
	X.mul(&x3, &x2)
	r.Infinity = false
	c.SetInt(7)
	c.set_add(&x3)
	c.sqrt(&r.Y)
	r.Y.Normalize()
	if r.Y.IsOdd() != odd {
		r.Y.Negate(&r.Y, 1)
	}
}


func (pk *XY) AddXY(a *XY) {
	var xyz XYZ_t
	xyz.set_ge(pk)
	xyz.add_ge(&xyz, a)
	pk.set_gej(&xyz)
}

/*
func (BitCurve *BitCurve) Add(x1, y1, x2, y2 *big.Int) (*big.Int, *big.Int) {
	Z := new(big.Int).SetInt64(1)
	return BitCurve.affineFromJacobian(BitCurve.addJacobian(x1, y1, Z, x2, y2, Z))
}
*/

// TODO: think about optimizing this one
func (pk *XY) Multi(k []byte) bool {
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
	return true
}


func (pk *XY) GetPublicKey(out []byte) {
	pk.X.GetB32(out[1:33])
	if len(out)==65 {
		out[0] = 0x04
		pk.Y.GetB32(out[33:65])
	} else {
		if pk.Y.IsOdd() {
			out[0] = 0x03
		} else {
			out[0] = 0x02
		}
	}
}
