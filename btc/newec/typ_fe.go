package newec

import (
	"fmt"
	"encoding/hex"
)

type fe_t struct {
	n [10]uint32
}

func (a *fe_t) String() string {
	var tmp [32]byte
	b := *a
	b.normalize()
	b.get_b32(tmp[:])
	return hex.EncodeToString(tmp[:])
}

func (a *fe_t) print(lab string) {
	fmt.Println(lab + ":", a.String())
}

func (r *fe_t) set_b32(a []byte) {
	r.n[0] = 0; r.n[1] = 0; r.n[2] = 0; r.n[3] = 0; r.n[4] = 0;
	r.n[5] = 0; r.n[6] = 0; r.n[7] = 0; r.n[8] = 0; r.n[9] = 0;
	for i:=uint(0); i<32; i++ {
		for j:=uint(0); j<4; j++ {
			limb := (8*i+2*j)/26
			shift := (8*i+2*j)%26
			r.n[limb] |= (uint32)((a[31-i] >> (2*j)) & 0x3) << shift
		}
	}
}

func (r *fe_t) set_bytes(a []byte) {
	if len(a)>32 {
		panic("too many bytes to set")
	}
	if len(a)==32 {
		r.set_b32(a)
	} else {
		var buf [32]byte
		copy(buf[32-len(a):], a)
		r.set_b32(buf[:])
	}
}

func (r *fe_t) set_hex(s string) {
	d, _ := hex.DecodeString(s)
	r.set_bytes(d)
}

func (a *fe_t) is_odd() bool {
	return (a.n[0]&1) != 0
}

func (a *fe_t) is_zero() bool {
	return (a.n[0] == 0 && a.n[1] == 0 && a.n[2] == 0 && a.n[3] == 0 && a.n[4] == 0 && a.n[5] == 0 && a.n[6] == 0 && a.n[7] == 0 && a.n[8] == 0 && a.n[9] == 0)
}


func (r *fe_t) set_int(a uint32) {
	r.n[0] = a; r.n[1] = 0; r.n[2] = 0; r.n[3] = 0; r.n[4] = 0;
	r.n[5] = 0; r.n[6] = 0; r.n[7] = 0; r.n[8] = 0; r.n[9] = 0;
}

func (r *fe_t) normalize() {
	c := r.n[0]
	t0 := c & 0x3FFFFFF
	c = (c >> 26) + r.n[1]
	t1 := c & 0x3FFFFFF
	c = (c >> 26) + r.n[2]
	t2 := c & 0x3FFFFFF
	c = (c >> 26) + r.n[3]
	t3 := c & 0x3FFFFFF
	c = (c >> 26) + r.n[4]
	t4 := c & 0x3FFFFFF
	c = (c >> 26) + r.n[5]
	t5 := c & 0x3FFFFFF
	c = (c >> 26) + r.n[6]
	t6 := c & 0x3FFFFFF
	c = (c >> 26) + r.n[7]
	t7 := c & 0x3FFFFFF
	c = (c >> 26) + r.n[8]
	t8 := c & 0x3FFFFFF
	c = (c >> 26) + r.n[9]
	t9 := c & 0x03FFFFF
	c >>= 22

	// The following code will not modify the t's if c is initially 0.
	d := c * 0x3D1 + t0
	t0 = d & 0x3FFFFFF
	d = (d >> 26) + t1 + c*0x40
	t1 = d & 0x3FFFFFF
	d = (d >> 26) + t2
	t2 = d & 0x3FFFFFF
	d = (d >> 26) + t3
	t3 = d & 0x3FFFFFF
	d = (d >> 26) + t4
	t4 = d & 0x3FFFFFF
	d = (d >> 26) + t5
	t5 = d & 0x3FFFFFF
	d = (d >> 26) + t6
	t6 = d & 0x3FFFFFF
	d = (d >> 26) + t7
	t7 = d & 0x3FFFFFF
	d = (d >> 26) + t8
	t8 = d & 0x3FFFFFF
	d = (d >> 26) + t9
	t9 = d & 0x03FFFFF

	// Subtract p if result >= p
	low := (uint64(t1) << 26) | uint64(t0)
	//mask := uint64(-(int64)((t9 < 0x03FFFFF) | (t8 < 0x3FFFFFF) | (t7 < 0x3FFFFFF) | (t6 < 0x3FFFFFF) | (t5 < 0x3FFFFFF) | (t4 < 0x3FFFFFF) | (t3 < 0x3FFFFFF) | (t2 < 0x3FFFFFF) | (low < 0xFFFFEFFFFFC2F)))
	var mask uint64
	if (t9 < 0x03FFFFF) ||
		(t8 < 0x3FFFFFF) ||
		(t7 < 0x3FFFFFF) ||
		(t6 < 0x3FFFFFF) ||
		(t5 < 0x3FFFFFF) ||
		(t4 < 0x3FFFFFF) ||
		(t3 < 0x3FFFFFF) ||
		(t2 < 0x3FFFFFF) ||
		(low < 0xFFFFEFFFFFC2F) {
		mask = 0xFFFFFFFFFFFFFFFF
	}
	t9 &= uint32(mask)
	t8 &= uint32(mask)
	t7 &= uint32(mask)
	t6 &= uint32(mask)
	t5 &= uint32(mask)
	t4 &= uint32(mask)
	t3 &= uint32(mask)
	t2 &= uint32(mask)
	low -= ((mask^0xFFFFFFFFFFFFFFFF) & 0xFFFFEFFFFFC2F)

	// push internal variables back
	r.n[0] = uint32(low) & 0x3FFFFFF
	r.n[1] = uint32(low >> 26) & 0x3FFFFFF
	r.n[2] = t2; r.n[3] = t3; r.n[4] = t4
	r.n[5] = t5; r.n[6] = t6; r.n[7] = t7;
	r.n[8] = t8; r.n[9] = t9
}

func (a *fe_t) get_b32(r []byte) {
	var i, j, c, limb, shift uint32
	for i=0; i<32; i++ {
		c = 0
		for j=0; j<4; j++ {
			limb = (8*i+2*j)/26
			shift =(8*i+2*j)%26
			c |= ((a.n[limb] >> shift) & 0x3) << (2 * j)
		}
		r[31-i] = byte(c)
	}
}

func (a *fe_t) equal(b *fe_t) bool {
	return (a.n[0] == b.n[0] && a.n[1] == b.n[1] && a.n[2] == b.n[2] && a.n[3] == b.n[3] && a.n[4] == b.n[4] &&
			a.n[5] == b.n[5] && a.n[6] == b.n[6] && a.n[7] == b.n[7] && a.n[8] == b.n[8] && a.n[9] == b.n[9])
}

func (r *fe_t) set_add(a *fe_t) {
	r.n[0] += a.n[0]
	r.n[1] += a.n[1]
	r.n[2] += a.n[2]
	r.n[3] += a.n[3]
	r.n[4] += a.n[4]
	r.n[5] += a.n[5]
	r.n[6] += a.n[6]
	r.n[7] += a.n[7]
	r.n[8] += a.n[8]
	r.n[9] += a.n[9]
}

func (r *fe_t) mul_int(a uint32) {
	r.n[0] *= a
	r.n[1] *= a
	r.n[2] *= a
	r.n[3] *= a
	r.n[4] *= a
	r.n[5] *= a
	r.n[6] *= a
	r.n[7] *= a
	r.n[8] *= a
	r.n[9] *= a
}


func (a *fe_t) negate(r *fe_t, m uint32) {
	r.n[0] = 0x3FFFC2F * (m + 1) - a.n[0]
	r.n[1] = 0x3FFFFBF * (m + 1) - a.n[1]
	r.n[2] = 0x3FFFFFF * (m + 1) - a.n[2]
	r.n[3] = 0x3FFFFFF * (m + 1) - a.n[3]
	r.n[4] = 0x3FFFFFF * (m + 1) - a.n[4]
	r.n[5] = 0x3FFFFFF * (m + 1) - a.n[5]
	r.n[6] = 0x3FFFFFF * (m + 1) - a.n[6]
	r.n[7] = 0x3FFFFFF * (m + 1) - a.n[7]
	r.n[8] = 0x3FFFFFF * (m + 1) - a.n[8]
	r.n[9] = 0x03FFFFF * (m + 1) - a.n[9]
}

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


func (a *fe_t) inv_var(r *fe_t) {
	var b [32]byte
	var c fe_t
	c = *a
	c.normalize()
	c.get_b32(b[:])
	var n num_t
	n.SetBytes(b[:])
	n.ModInverse(n.big(), secp256k1.P)
	r.set_bytes(n.Bytes())
}

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
