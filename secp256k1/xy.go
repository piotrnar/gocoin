package secp256k1

import (
	"fmt"
)


type XY struct {
	X, Y Field
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


func (r *XY) SetXY(X, Y *Field) {
	r.Infinity = false
	r.X = *X
	r.Y = *Y
}


func (a *XY) IsValid() bool {
	if a.Infinity {
		return false
	}
	var y2, x3, c Field
	a.Y.Sqr(&y2)
	a.X.Sqr(&x3); x3.Mul(&x3, &a.X)
	c.SetInt(7)
	x3.SetAdd(&c)
	y2.Normalize()
	x3.Normalize()
	return y2.Equals(&x3)
}


func (r *XY) set_gej(a *XYZ) {
	var z2, z3 Field;
	a.Z.InvVar(&a.Z)
	a.Z.Sqr(&z2)
	a.Z.Mul(&z3, &z2)
	a.X.Mul(&a.X, &z2)
	a.Y.Mul(&a.Y, &z3)
	a.Z.SetInt(1)
	r.Infinity = a.Infinity
	r.X = a.X
	r.Y = a.Y
}

func (a *XY) precomp(w int) (pre []XY) {
	pre = make([]XY, (1 << (uint(w)-2)))
	pre[0] = *a;
	var X, d, tmp XYZ
	X.set_ge(a)
	X.Double(&d)
	for i:=1 ; i<len(pre); i++ {
		d.AddXY(&tmp, &pre[i-1])
		pre[i].set_gej(&tmp)
	}
	return
}

func (a *XY) Neg(r *XY) {
	r.Infinity = a.Infinity
	r.X = a.X
	r.Y = a.Y
	r.Y.Normalize()
	r.Y.Negate(&r.Y, 1)
}


func (r *XY) set_xo(X *Field, odd bool) {
	var c, x2, x3 Field
	r.X = *X
	X.Sqr(&x2)
	X.Mul(&x3, &x2)
	r.Infinity = false
	c.SetInt(7)
	c.SetAdd(&x3)
	c.Sqrt(&r.Y)
	r.Y.Normalize()
	if r.Y.IsOdd() != odd {
		r.Y.Negate(&r.Y, 1)
	}
}


func (pk *XY) AddXY(a *XY) {
	var xyz XYZ
	xyz.set_ge(pk)
	xyz.AddXY(&xyz, a)
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
	var B, r XYZ

	B.set_ge(pk)
	r = B

	seen := false
	for _, byte := range k {
		for bitNum := 0; bitNum < 8; bitNum++ {
			if seen {
				r.Double(&r)
			}
			if byte&0x80 == 0x80 {
				if !seen {
					seen = true
				} else {
					r.Add(&r, &B)
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
