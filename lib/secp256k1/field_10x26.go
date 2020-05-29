// +build !amd64,!arm64,!arm64be,!ppc64,!ppc64le,!mips64,!mips64le,!s390x,!sparc64

package secp256k1

import (
)

const FieldArch = "10x26"

type Field struct {
	n [10]uint32
}

func (r *Field) SetB32(a []byte) {
//println("SetB32")
	r.n[0] = uint32(a[31]) | (uint32(a[30]) << 8) | (uint32(a[29]) << 16) | (uint32(a[28] & 0x3) << 24)
	r.n[1] = uint32((a[28] >> 2) & 0x3f) | (uint32(a[27]) << 6) | (uint32(a[26]) << 14) | (uint32(a[25] & 0xf) << 22)
	r.n[2] = uint32((a[25] >> 4) & 0xf) | (uint32(a[24]) << 4) | (uint32(a[23]) << 12) | (uint32(a[22] & 0x3f) << 20)
	r.n[3] = uint32((a[22] >> 6) & 0x3) | (uint32(a[21]) << 2) | (uint32(a[20]) << 10) | (uint32(a[19]) << 18)
	r.n[4] = uint32(a[18]) | (uint32(a[17]) << 8) | (uint32(a[16]) << 16) | (uint32(a[15] & 0x3) << 24)
	r.n[5] = uint32((a[15] >> 2) & 0x3f) | (uint32(a[14]) << 6) | (uint32(a[13]) << 14) | (uint32(a[12] & 0xf) << 22)
	r.n[6] = uint32((a[12] >> 4) & 0xf) | (uint32(a[11]) << 4) | (uint32(a[10]) << 12) | (uint32(a[9] & 0x3f) << 20)
	r.n[7] = uint32((a[9] >> 6) & 0x3) | (uint32(a[8]) << 2) | (uint32(a[7]) << 10) | (uint32(a[6]) << 18)
	r.n[8] = uint32(a[5]) | (uint32(a[4]) << 8) | (uint32(a[3]) << 16) | (uint32(a[2] & 0x3) << 24)
	r.n[9] = uint32((a[2] >> 2) & 0x3f) | (uint32(a[1]) << 6) | (uint32(a[0]) << 14)
}

func (a *Field) IsZero() bool {
//println("IsZero")
	return (a.n[0] == 0 && a.n[1] == 0 && a.n[2] == 0 && a.n[3] == 0 && a.n[4] == 0 && a.n[5] == 0 && a.n[6] == 0 && a.n[7] == 0 && a.n[8] == 0 && a.n[9] == 0)
}


func (r *Field) SetInt(a uint32) {
//println("SetInt")
	r.n[0] = a; r.n[1] = 0; r.n[2] = 0; r.n[3] = 0; r.n[4] = 0;
	r.n[5] = 0; r.n[6] = 0; r.n[7] = 0; r.n[8] = 0; r.n[9] = 0;
}

func (r *Field) Normalize() {
//println("Normalize")
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

func (a *Field) GetB32(r []byte) {
//println("GetB32")
	r[0] = byte(a.n[9] >> 14)
	r[1] = byte(a.n[9] >> 6)
	r[2] = byte((a.n[9] & 0x3F) << 2) | byte((a.n[8] >> 24) & 0x3)
	r[3] = byte(a.n[8] >> 16)
	r[4] = byte(a.n[8] >> 8)
	r[5] = byte(a.n[8])
	r[6] = byte(a.n[7] >> 18)
	r[7] = byte(a.n[7] >> 10)
	r[8] = byte(a.n[7] >> 2)
	r[9] = byte((a.n[7] & 0x3) << 6) | byte((a.n[6] >> 20) & 0x3f)
	r[10] = byte(a.n[6] >> 12)
	r[11] = byte(a.n[6] >> 4)
	r[12] = byte((a.n[6] & 0xf) << 4) | byte((a.n[5] >> 22) & 0xf)
	r[13] = byte(a.n[5] >> 14)
	r[14] = byte(a.n[5] >> 6)
	r[15] = byte((a.n[5] & 0x3f) << 2) | byte((a.n[4] >> 24) & 0x3)
	r[16] = byte(a.n[4] >> 16)
	r[17] = byte(a.n[4] >> 8)
	r[18] = byte(a.n[4])
	r[19] = byte(a.n[3] >> 18)
	r[20] = byte(a.n[3] >> 10)
	r[21] = byte(a.n[3] >> 2)
	r[22] = byte((a.n[3] & 0x3) << 6) | byte((a.n[2] >> 20) & 0x3f)
	r[23] = byte(a.n[2] >> 12)
	r[24] = byte(a.n[2] >> 4)
	r[25] = byte((a.n[2] & 0xf) << 4) | byte((a.n[1] >> 22) & 0xf)
	r[26] = byte(a.n[1] >> 14)
	r[27] = byte(a.n[1] >> 6)
	r[28] = byte((a.n[1] & 0x3f) << 2) | byte((a.n[0] >> 24) & 0x3)
	r[29] = byte(a.n[0] >> 16)
	r[30] = byte(a.n[0] >> 8)
	r[31] = byte(a.n[0])
}

func (a *Field) Equals(b *Field) bool {
//println("Equals")
	return (a.n[0] == b.n[0] && a.n[1] == b.n[1] && a.n[2] == b.n[2] && a.n[3] == b.n[3] && a.n[4] == b.n[4] &&
			a.n[5] == b.n[5] && a.n[6] == b.n[6] && a.n[7] == b.n[7] && a.n[8] == b.n[8] && a.n[9] == b.n[9])
}

func (r *Field) SetAdd(a *Field) {
//println("SetAdd")
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

func (r *Field) MulInt(a uint32) {
//println("MulInt")
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


func (a *Field) Negate(r *Field, m uint32) {
//println("Negate")
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


func (a *Field) Mul(r, b *Field) {
//println("Mul", a.String(), b.String())
	var c, d uint64
	var t0, t1, t2, t3, t4, t5, t6 uint64
	var t7, t8, t9, t10, t11, t12, t13 uint64
	var t14, t15, t16, t17, t18, t19 uint64

	c = uint64(a.n[0]) * uint64(b.n[0])
	t0 = c & 0x3FFFFFF
	c = c >> 26
	c = c + uint64(a.n[0])*uint64(b.n[1]) +
		uint64(a.n[1])*uint64(b.n[0])
	t1 = c & 0x3FFFFFF
	c = c >> 26
	c = c + uint64(a.n[0])*uint64(b.n[2]) +
		uint64(a.n[1])*uint64(b.n[1]) +
		uint64(a.n[2])*uint64(b.n[0])
	t2 = c & 0x3FFFFFF
	c = c >> 26
	c = c + uint64(a.n[0])*uint64(b.n[3]) +
		uint64(a.n[1])*uint64(b.n[2]) +
		uint64(a.n[2])*uint64(b.n[1]) +
		uint64(a.n[3])*uint64(b.n[0])
	t3 = c & 0x3FFFFFF
	c = c >> 26
	c = c + uint64(a.n[0])*uint64(b.n[4]) +
		uint64(a.n[1])*uint64(b.n[3]) +
		uint64(a.n[2])*uint64(b.n[2]) +
		uint64(a.n[3])*uint64(b.n[1]) +
		uint64(a.n[4])*uint64(b.n[0])
	t4 = c & 0x3FFFFFF
	c = c >> 26
	c = c + uint64(a.n[0])*uint64(b.n[5]) +
		uint64(a.n[1])*uint64(b.n[4]) +
		uint64(a.n[2])*uint64(b.n[3]) +
		uint64(a.n[3])*uint64(b.n[2]) +
		uint64(a.n[4])*uint64(b.n[1]) +
		uint64(a.n[5])*uint64(b.n[0])
	t5 = c & 0x3FFFFFF
	c = c >> 26
	c = c + uint64(a.n[0])*uint64(b.n[6]) +
		uint64(a.n[1])*uint64(b.n[5]) +
		uint64(a.n[2])*uint64(b.n[4]) +
		uint64(a.n[3])*uint64(b.n[3]) +
		uint64(a.n[4])*uint64(b.n[2]) +
		uint64(a.n[5])*uint64(b.n[1]) +
		uint64(a.n[6])*uint64(b.n[0])
	t6 = c & 0x3FFFFFF
	c = c >> 26
	c = c + uint64(a.n[0])*uint64(b.n[7]) +
		uint64(a.n[1])*uint64(b.n[6]) +
		uint64(a.n[2])*uint64(b.n[5]) +
		uint64(a.n[3])*uint64(b.n[4]) +
		uint64(a.n[4])*uint64(b.n[3]) +
		uint64(a.n[5])*uint64(b.n[2]) +
		uint64(a.n[6])*uint64(b.n[1]) +
		uint64(a.n[7])*uint64(b.n[0])
	t7 = c & 0x3FFFFFF
	c = c >> 26
	c = c + uint64(a.n[0])*uint64(b.n[8]) +
		uint64(a.n[1])*uint64(b.n[7]) +
		uint64(a.n[2])*uint64(b.n[6]) +
		uint64(a.n[3])*uint64(b.n[5]) +
		uint64(a.n[4])*uint64(b.n[4]) +
		uint64(a.n[5])*uint64(b.n[3]) +
		uint64(a.n[6])*uint64(b.n[2]) +
		uint64(a.n[7])*uint64(b.n[1]) +
		uint64(a.n[8])*uint64(b.n[0])
	t8 = c & 0x3FFFFFF
	c = c >> 26
	c = c + uint64(a.n[0])*uint64(b.n[9]) +
		uint64(a.n[1])*uint64(b.n[8]) +
		uint64(a.n[2])*uint64(b.n[7]) +
		uint64(a.n[3])*uint64(b.n[6]) +
		uint64(a.n[4])*uint64(b.n[5]) +
		uint64(a.n[5])*uint64(b.n[4]) +
		uint64(a.n[6])*uint64(b.n[3]) +
		uint64(a.n[7])*uint64(b.n[2]) +
		uint64(a.n[8])*uint64(b.n[1]) +
		uint64(a.n[9])*uint64(b.n[0])
	t9 = c & 0x3FFFFFF
	c = c >> 26
	c = c + uint64(a.n[1])*uint64(b.n[9]) +
		uint64(a.n[2])*uint64(b.n[8]) +
		uint64(a.n[3])*uint64(b.n[7]) +
		uint64(a.n[4])*uint64(b.n[6]) +
		uint64(a.n[5])*uint64(b.n[5]) +
		uint64(a.n[6])*uint64(b.n[4]) +
		uint64(a.n[7])*uint64(b.n[3]) +
		uint64(a.n[8])*uint64(b.n[2]) +
		uint64(a.n[9])*uint64(b.n[1])
	t10 = c & 0x3FFFFFF
	c = c >> 26
	c = c + uint64(a.n[2])*uint64(b.n[9]) +
		uint64(a.n[3])*uint64(b.n[8]) +
		uint64(a.n[4])*uint64(b.n[7]) +
		uint64(a.n[5])*uint64(b.n[6]) +
		uint64(a.n[6])*uint64(b.n[5]) +
		uint64(a.n[7])*uint64(b.n[4]) +
		uint64(a.n[8])*uint64(b.n[3]) +
		uint64(a.n[9])*uint64(b.n[2])
	t11 = c & 0x3FFFFFF
	c = c >> 26
	c = c + uint64(a.n[3])*uint64(b.n[9]) +
		uint64(a.n[4])*uint64(b.n[8]) +
		uint64(a.n[5])*uint64(b.n[7]) +
		uint64(a.n[6])*uint64(b.n[6]) +
		uint64(a.n[7])*uint64(b.n[5]) +
		uint64(a.n[8])*uint64(b.n[4]) +
		uint64(a.n[9])*uint64(b.n[3])
	t12 = c & 0x3FFFFFF
	c = c >> 26
	c = c + uint64(a.n[4])*uint64(b.n[9]) +
		uint64(a.n[5])*uint64(b.n[8]) +
		uint64(a.n[6])*uint64(b.n[7]) +
		uint64(a.n[7])*uint64(b.n[6]) +
		uint64(a.n[8])*uint64(b.n[5]) +
		uint64(a.n[9])*uint64(b.n[4])
	t13 = c & 0x3FFFFFF
	c = c >> 26
	c = c + uint64(a.n[5])*uint64(b.n[9]) +
		uint64(a.n[6])*uint64(b.n[8]) +
		uint64(a.n[7])*uint64(b.n[7]) +
		uint64(a.n[8])*uint64(b.n[6]) +
		uint64(a.n[9])*uint64(b.n[5])
	t14 = c & 0x3FFFFFF
	c = c >> 26
	c = c + uint64(a.n[6])*uint64(b.n[9]) +
		uint64(a.n[7])*uint64(b.n[8]) +
		uint64(a.n[8])*uint64(b.n[7]) +
		uint64(a.n[9])*uint64(b.n[6])
	t15 = c & 0x3FFFFFF
	c = c >> 26
	c = c + uint64(a.n[7])*uint64(b.n[9]) +
		uint64(a.n[8])*uint64(b.n[8]) +
		uint64(a.n[9])*uint64(b.n[7])
	t16 = c & 0x3FFFFFF
	c = c >> 26
	c = c + uint64(a.n[8])*uint64(b.n[9]) +
		uint64(a.n[9])*uint64(b.n[8])
	t17 = c & 0x3FFFFFF
	c = c >> 26
	c = c + uint64(a.n[9])*uint64(b.n[9])
	t18 = c & 0x3FFFFFF
	c = c >> 26
	t19 = c

	c = t0 + t10*0x3D10
	t0 = c & 0x3FFFFFF
	c = c >> 26
	c = c + t1 + t10*0x400 + t11*0x3D10
	t1 = c & 0x3FFFFFF
	c = c >> 26
	c = c + t2 + t11*0x400 + t12*0x3D10
	t2 = c & 0x3FFFFFF
	c = c >> 26
	c = c + t3 + t12*0x400 + t13*0x3D10
	r.n[3] = uint32(c) & 0x3FFFFFF
	c = c >> 26
	c = c + t4 + t13*0x400 + t14*0x3D10
	r.n[4] = uint32(c) & 0x3FFFFFF
	c = c >> 26
	c = c + t5 + t14*0x400 + t15*0x3D10
	r.n[5] = uint32(c) & 0x3FFFFFF
	c = c >> 26
	c = c + t6 + t15*0x400 + t16*0x3D10
	r.n[6] = uint32(c) & 0x3FFFFFF
	c = c >> 26
	c = c + t7 + t16*0x400 + t17*0x3D10
	r.n[7] = uint32(c) & 0x3FFFFFF
	c = c >> 26
	c = c + t8 + t17*0x400 + t18*0x3D10
	r.n[8] = uint32(c) & 0x3FFFFFF
	c = c >> 26
	c = c + t9 + t18*0x400 + t19*0x1000003D10
	r.n[9] = uint32(c) & 0x03FFFFF
	c = c >> 22
	d = t0 + c*0x3D1
	r.n[0] = uint32(d) & 0x3FFFFFF
	d = d >> 26
	d = d + t1 + c*0x40
	r.n[1] = uint32(d) & 0x3FFFFFF
	d = d >> 26
	r.n[2] = uint32(t2 + d)
//println("Mul:", r.String())
	r.Normalize()
//println("Mul:", r.String())
	//os.Exit(0)
}

func (a *Field) Sqr(r *Field) {
	var c, d uint64
	var t0, t1, t2, t3, t4, t5, t6 uint64
	var t7, t8, t9, t10, t11, t12, t13 uint64
	var t14, t15, t16, t17, t18, t19 uint64

	c = uint64(a.n[0]) * uint64(a.n[0]);
	t0 = c & 0x3FFFFFF; c = c >> 26;
	c = c + (uint64(a.n[0])*2) * uint64(a.n[1]);
	t1 = c & 0x3FFFFFF; c = c >> 26;
	c = c + (uint64(a.n[0])*2) * uint64(a.n[2]) +
	        uint64(a.n[1]) * uint64(a.n[1]);
	t2 = c & 0x3FFFFFF; c = c >> 26;
	c = c + (uint64(a.n[0])*2) * uint64(a.n[3]) +
	        (uint64(a.n[1])*2) * uint64(a.n[2]);
	t3 = c & 0x3FFFFFF; c = c >> 26;
	c = c + (uint64(a.n[0])*2) * uint64(a.n[4]) +
	        (uint64(a.n[1])*2) * uint64(a.n[3]) +
	        uint64(a.n[2]) * uint64(a.n[2]);
	t4 = c & 0x3FFFFFF; c = c >> 26;
	c = c + (uint64(a.n[0])*2) * uint64(a.n[5]) +
	        (uint64(a.n[1])*2) * uint64(a.n[4]) +
	        (uint64(a.n[2])*2) * uint64(a.n[3]);
	t5 = c & 0x3FFFFFF; c = c >> 26;
	c = c + (uint64(a.n[0])*2) * uint64(a.n[6]) +
	        (uint64(a.n[1])*2) * uint64(a.n[5]) +
	        (uint64(a.n[2])*2) * uint64(a.n[4]) +
	        uint64(a.n[3]) * uint64(a.n[3]);
	t6 = c & 0x3FFFFFF; c = c >> 26;
	c = c + (uint64(a.n[0])*2) * uint64(a.n[7]) +
	        (uint64(a.n[1])*2) * uint64(a.n[6]) +
	        (uint64(a.n[2])*2) * uint64(a.n[5]) +
	        (uint64(a.n[3])*2) * uint64(a.n[4]);
	t7 = c & 0x3FFFFFF; c = c >> 26;
	c = c + (uint64(a.n[0])*2) * uint64(a.n[8]) +
	        (uint64(a.n[1])*2) * uint64(a.n[7]) +
	        (uint64(a.n[2])*2) * uint64(a.n[6]) +
	        (uint64(a.n[3])*2) * uint64(a.n[5]) +
	        uint64(a.n[4]) * uint64(a.n[4]);
	t8 = c & 0x3FFFFFF; c = c >> 26;
	c = c + (uint64(a.n[0])*2) * uint64(a.n[9]) +
	        (uint64(a.n[1])*2) * uint64(a.n[8]) +
	        (uint64(a.n[2])*2) * uint64(a.n[7]) +
	        (uint64(a.n[3])*2) * uint64(a.n[6]) +
	        (uint64(a.n[4])*2) * uint64(a.n[5]);
	t9 = c & 0x3FFFFFF; c = c >> 26;
	c = c + (uint64(a.n[1])*2) * uint64(a.n[9]) +
	        (uint64(a.n[2])*2) * uint64(a.n[8]) +
	        (uint64(a.n[3])*2) * uint64(a.n[7]) +
	        (uint64(a.n[4])*2) * uint64(a.n[6]) +
	        uint64(a.n[5]) * uint64(a.n[5]);
	t10 = c & 0x3FFFFFF; c = c >> 26;
	c = c + (uint64(a.n[2])*2) * uint64(a.n[9]) +
	        (uint64(a.n[3])*2) * uint64(a.n[8]) +
	        (uint64(a.n[4])*2) * uint64(a.n[7]) +
	        (uint64(a.n[5])*2) * uint64(a.n[6]);
	t11 = c & 0x3FFFFFF; c = c >> 26;
	c = c + (uint64(a.n[3])*2) * uint64(a.n[9]) +
	        (uint64(a.n[4])*2) * uint64(a.n[8]) +
	        (uint64(a.n[5])*2) * uint64(a.n[7]) +
	        uint64(a.n[6]) * uint64(a.n[6]);
	t12 = c & 0x3FFFFFF; c = c >> 26;
	c = c + (uint64(a.n[4])*2) * uint64(a.n[9]) +
	        (uint64(a.n[5])*2) * uint64(a.n[8]) +
	        (uint64(a.n[6])*2) * uint64(a.n[7]);
	t13 = c & 0x3FFFFFF; c = c >> 26;
	c = c + (uint64(a.n[5])*2) * uint64(a.n[9]) +
	        (uint64(a.n[6])*2) * uint64(a.n[8]) +
	        uint64(a.n[7]) * uint64(a.n[7]);
	t14 = c & 0x3FFFFFF; c = c >> 26;
	c = c + (uint64(a.n[6])*2) * uint64(a.n[9]) +
	        (uint64(a.n[7])*2) * uint64(a.n[8]);
	t15 = c & 0x3FFFFFF; c = c >> 26;
	c = c + (uint64(a.n[7])*2) * uint64(a.n[9]) +
	        uint64(a.n[8]) * uint64(a.n[8]);
	t16 = c & 0x3FFFFFF; c = c >> 26;
	c = c + (uint64(a.n[8])*2) * uint64(a.n[9]);
	t17 = c & 0x3FFFFFF; c = c >> 26;
	c = c + uint64(a.n[9]) * uint64(a.n[9]);
	t18 = c & 0x3FFFFFF; c = c >> 26;
	t19 = c;

	c = t0 + t10 * 0x3D10;
	t0 = c & 0x3FFFFFF; c = c >> 26;
	c = c + t1 + t10*0x400 + t11 * 0x3D10;
	t1 = c & 0x3FFFFFF; c = c >> 26;
	c = c + t2 + t11*0x400 + t12 * 0x3D10;
	t2 = c & 0x3FFFFFF; c = c >> 26;
	c = c + t3 + t12*0x400 + t13 * 0x3D10;
	r.n[3] = uint32(c) & 0x3FFFFFF; c = c >> 26;
	c = c + t4 + t13*0x400 + t14 * 0x3D10;
	r.n[4] = uint32(c) & 0x3FFFFFF; c = c >> 26;
	c = c + t5 + t14*0x400 + t15 * 0x3D10;
	r.n[5] = uint32(c) & 0x3FFFFFF; c = c >> 26;
	c = c + t6 + t15*0x400 + t16 * 0x3D10;
	r.n[6] = uint32(c) & 0x3FFFFFF; c = c >> 26;
	c = c + t7 + t16*0x400 + t17 * 0x3D10;
	r.n[7] = uint32(c) & 0x3FFFFFF; c = c >> 26;
	c = c + t8 + t17*0x400 + t18 * 0x3D10;
	r.n[8] = uint32(c) & 0x3FFFFFF; c = c >> 26;
	c = c + t9 + t18*0x400 + t19 * 0x1000003D10;
	r.n[9] = uint32(c) & 0x03FFFFF; c = c >> 22;
	d = t0 + c * 0x3D1;
	r.n[0] = uint32(d) & 0x3FFFFFF; d = d >> 26;
	d = d + t1 + c*0x40;
	r.n[1] = uint32(d) & 0x3FFFFFF; d = d >> 26;
	r.n[2] = uint32(t2 + d)
}
