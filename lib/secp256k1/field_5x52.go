//go:build amd64 || arm64 || arm64be || ppc64 || ppc64le || mips64 || mips64le || s390x || sparc64
// +build amd64 arm64 arm64be ppc64 ppc64le mips64 mips64le s390x sparc64

package secp256k1

import (
	"math/bits"
)

const FieldArch = "5x52"

const (
	M = 0xFFFFFFFFFFFFF
	R = 0x1000003D10
)

type Field struct {
	n [5]uint64
}

func (r *Field) SetB32(a []byte) {
	r.n[0] = uint64(a[31]) | (uint64(a[30]) << 8) | (uint64(a[29]) << 16) |
		(uint64(a[28]) << 24) | (uint64(a[27]) << 32) | (uint64(a[26]) << 40) | ((uint64(a[25]) & 0xF) << 48)

	r.n[1] = ((uint64(a[25]) >> 4) & 0xF) | (uint64(a[24]) << 4) | (uint64(a[23]) << 12) | (uint64(a[22]) << 20) |
		(uint64(a[21]) << 28) | (uint64(a[20]) << 36) | (uint64(a[19]) << 44)

	r.n[2] = uint64(a[18]) | (uint64(a[17]) << 8) | (uint64(a[16]) << 16) | (uint64(a[15]) << 24) |
		(uint64(a[14]) << 32) | (uint64(a[13]) << 40) | ((uint64(a[12]) & 0xF) << 48)

	r.n[3] = ((uint64(a[12]) >> 4) & 0xF) | (uint64(a[11]) << 4) | (uint64(a[10]) << 12) |
		(uint64(a[9]) << 20) | (uint64(a[8]) << 28) | (uint64(a[7]) << 36) | (uint64(a[6]) << 44)

	r.n[4] = uint64(a[5]) | (uint64(a[4]) << 8) | (uint64(a[3]) << 16) | (uint64(a[2]) << 24) |
		(uint64(a[1]) << 32) | (uint64(a[0]) << 40)
}

func (a *Field) IsZero() bool {
	return (a.n[0] == 0 && a.n[1] == 0 && a.n[2] == 0 && a.n[3] == 0 && a.n[4] == 0)
}

func (r *Field) SetInt(a uint64) {
	r.n[0] = a
	r.n[1] = 0
	r.n[2] = 0
	r.n[3] = 0
	r.n[4] = 0
}

func (r *Field) Normalize() {
	t0 := r.n[0]
	t1 := r.n[1]
	t2 := r.n[2]
	t3 := r.n[3]
	t4 := r.n[4]

	/* Reduce t4 at the start so there will be at most a single carry from the first pass */
	var m uint64
	x := t4 >> 48
	t4 &= 0x0FFFFFFFFFFFF

	/* The first pass ensures the magnitude is 1, ... */
	t0 += x * 0x1000003D1
	t1 += (t0 >> 52)
	t0 &= 0xFFFFFFFFFFFFF
	t2 += (t1 >> 52)
	t1 &= 0xFFFFFFFFFFFFF
	m = t1
	t3 += (t2 >> 52)
	t2 &= 0xFFFFFFFFFFFFF
	m &= t2
	t4 += (t3 >> 52)
	t3 &= 0xFFFFFFFFFFFFF
	m &= t3

	/* At most a single final reduction is needed; check if the value is >= the field characteristic */
	x = (t4 >> 48)
	if (t4 == 0x0FFFFFFFFFFFF) && (m == 0xFFFFFFFFFFFFF) && (t0 >= 0xFFFFEFFFFFC2F) {
		x |= 1
	}

	/* Apply the final reduction (for constant-time behaviour, we do it always) */
	t0 += x * 0x1000003D1
	t1 += (t0 >> 52)
	t0 &= 0xFFFFFFFFFFFFF
	t2 += (t1 >> 52)
	t1 &= 0xFFFFFFFFFFFFF
	t3 += (t2 >> 52)
	t2 &= 0xFFFFFFFFFFFFF
	t4 += (t3 >> 52)
	t3 &= 0xFFFFFFFFFFFFF

	/* Mask off the possible multiple of 2^256 from the final reduction */
	t4 &= 0x0FFFFFFFFFFFF

	r.n[0] = t0
	r.n[1] = t1
	r.n[2] = t2
	r.n[3] = t3
	r.n[4] = t4
}

func (a *Field) GetB32(r []byte) {
	r[0] = byte(a.n[4] >> 40)
	r[1] = byte(a.n[4] >> 32)
	r[2] = byte(a.n[4] >> 24)
	r[3] = byte(a.n[4] >> 16)
	r[4] = byte(a.n[4] >> 8)
	r[5] = byte(a.n[4])
	r[6] = byte(a.n[3] >> 44)
	r[7] = byte(a.n[3] >> 36)
	r[8] = byte(a.n[3] >> 28)
	r[9] = byte(a.n[3] >> 20)
	r[10] = byte(a.n[3] >> 12)
	r[11] = byte(a.n[3] >> 4)
	r[12] = (byte(a.n[2]>>48) & 0xF) | (byte(a.n[3]&0xF) << 4)
	r[13] = byte(a.n[2] >> 40)
	r[14] = byte(a.n[2] >> 32)
	r[15] = byte(a.n[2] >> 24)
	r[16] = byte(a.n[2] >> 16)
	r[17] = byte(a.n[2] >> 8)
	r[18] = byte(a.n[2])
	r[19] = byte(a.n[1] >> 44)
	r[20] = byte(a.n[1] >> 36)
	r[21] = byte(a.n[1] >> 28)
	r[22] = byte(a.n[1] >> 20)
	r[23] = byte(a.n[1] >> 12)
	r[24] = byte(a.n[1] >> 4)
	r[25] = (byte(a.n[0]>>48) & 0xF) | (byte(a.n[1]&0xF) << 4)
	r[26] = byte(a.n[0] >> 40)
	r[27] = byte(a.n[0] >> 32)
	r[28] = byte(a.n[0] >> 24)
	r[29] = byte(a.n[0] >> 16)
	r[30] = byte(a.n[0] >> 8)
	r[31] = byte(a.n[0])
}

func (a *Field) Equals(b *Field) bool {
	return (a.n[0] == b.n[0] && a.n[1] == b.n[1] && a.n[2] == b.n[2] && a.n[3] == b.n[3] && a.n[4] == b.n[4])
}

func (r *Field) SetAdd(a *Field) {
	r.n[0] += a.n[0]
	r.n[1] += a.n[1]
	r.n[2] += a.n[2]
	r.n[3] += a.n[3]
	r.n[4] += a.n[4]
}

func (r *Field) MulInt(a uint64) {
	r.n[0] *= a
	r.n[1] *= a
	r.n[2] *= a
	r.n[3] *= a
	r.n[4] *= a
}

func (a *Field) Negate(r *Field, m uint64) {
	r.n[0] = 0xFFFFEFFFFFC2F*2*(m+1) - a.n[0]
	r.n[1] = 0xFFFFFFFFFFFFF*2*(m+1) - a.n[1]
	r.n[2] = 0xFFFFFFFFFFFFF*2*(m+1) - a.n[2]
	r.n[3] = 0xFFFFFFFFFFFFF*2*(m+1) - a.n[3]
	r.n[4] = 0x0FFFFFFFFFFFF*2*(m+1) - a.n[4]
}

func (a *Field) Mul(r, b *Field) {
	a0, a1, a2, a3, a4 := a.n[0], a.n[1], a.n[2], a.n[3], a.n[4]
	b0, b1, b2, b3, b4 := b.n[0], b.n[1], b.n[2], b.n[3], b.n[4]

	var c_lo, c_hi, d_lo, d_hi uint64
	var t3, t4, tx, u0 uint64
	var hi, lo, carry uint64

	// d = (uint128_t)a0 * b[3]
	d_hi, d_lo = bits.Mul64(a0, b3)

	// d += (uint128_t)a1 * b[2]
	hi, lo = bits.Mul64(a1, b2)
	d_lo, carry = bits.Add64(d_lo, lo, 0)
	d_hi, _ = bits.Add64(d_hi, hi, carry)

	// d += (uint128_t)a2 * b[1]
	hi, lo = bits.Mul64(a2, b1)
	d_lo, carry = bits.Add64(d_lo, lo, 0)
	d_hi, _ = bits.Add64(d_hi, hi, carry)

	// d += (uint128_t)a3 * b[0]
	hi, lo = bits.Mul64(a3, b0)
	d_lo, carry = bits.Add64(d_lo, lo, 0)
	d_hi, _ = bits.Add64(d_hi, hi, carry)

	// c = (uint128_t)a4 * b[4]
	c_hi, c_lo = bits.Mul64(a4, b4)

	// d += (uint128_t)(uint64_t)c * R
	hi, lo = bits.Mul64(c_lo, R)
	d_lo, carry = bits.Add64(d_lo, lo, 0)
	d_hi, _ = bits.Add64(d_hi, hi, carry)

	// c >>= 64
	// After shift, c contains only the high 64 bits

	// t3 = (uint64_t)d & M
	t3 = d_lo & M

	// d >>= 52
	d_lo = (d_lo >> 52) | (d_hi << 12)
	d_hi >>= 52

	// d += (uint128_t)a0 * b[4]
	hi, lo = bits.Mul64(a0, b4)
	d_lo, carry = bits.Add64(d_lo, lo, 0)
	d_hi, _ = bits.Add64(d_hi, hi, carry)

	// d += (uint128_t)a1 * b[3]
	hi, lo = bits.Mul64(a1, b3)
	d_lo, carry = bits.Add64(d_lo, lo, 0)
	d_hi, _ = bits.Add64(d_hi, hi, carry)

	// d += (uint128_t)a2 * b[2]
	hi, lo = bits.Mul64(a2, b2)
	d_lo, carry = bits.Add64(d_lo, lo, 0)
	d_hi, _ = bits.Add64(d_hi, hi, carry)

	// d += (uint128_t)a3 * b[1]
	hi, lo = bits.Mul64(a3, b1)
	d_lo, carry = bits.Add64(d_lo, lo, 0)
	d_hi, _ = bits.Add64(d_hi, hi, carry)

	// d += (uint128_t)a4 * b[0]
	hi, lo = bits.Mul64(a4, b0)
	d_lo, carry = bits.Add64(d_lo, lo, 0)
	d_hi, _ = bits.Add64(d_hi, hi, carry)

	// d += (uint128_t)(R << 12) * (uint64_t)c
	hi, lo = bits.Mul64(R<<12, c_hi)
	d_lo, carry = bits.Add64(d_lo, lo, 0)
	d_hi, _ = bits.Add64(d_hi, hi, carry)

	// t4 = (uint64_t)d & M
	t4 = d_lo & M

	// d >>= 52
	d_lo = (d_lo >> 52) | (d_hi << 12)
	d_hi >>= 52

	// tx = (t4 >> 48)
	tx = t4 >> 48

	// t4 &= (M >> 4)
	t4 &= (M >> 4)

	// c = (uint128_t)a0 * b[0]
	c_hi, c_lo = bits.Mul64(a0, b0)

	// d += (uint128_t)a1 * b[4]
	hi, lo = bits.Mul64(a1, b4)
	d_lo, carry = bits.Add64(d_lo, lo, 0)
	d_hi, _ = bits.Add64(d_hi, hi, carry)

	// d += (uint128_t)a2 * b[3]
	hi, lo = bits.Mul64(a2, b3)
	d_lo, carry = bits.Add64(d_lo, lo, 0)
	d_hi, _ = bits.Add64(d_hi, hi, carry)

	// d += (uint128_t)a3 * b[2]
	hi, lo = bits.Mul64(a3, b2)
	d_lo, carry = bits.Add64(d_lo, lo, 0)
	d_hi, _ = bits.Add64(d_hi, hi, carry)

	// d += (uint128_t)a4 * b[1]
	hi, lo = bits.Mul64(a4, b1)
	d_lo, carry = bits.Add64(d_lo, lo, 0)
	d_hi, _ = bits.Add64(d_hi, hi, carry)

	// u0 = (uint64_t)d & M
	u0 = d_lo & M

	// d >>= 52
	d_lo = (d_lo >> 52) | (d_hi << 12)
	d_hi >>= 52

	// u0 = (u0 << 4) | tx
	u0 = (u0 << 4) | tx

	// c += (uint128_t)u0 * (R >> 4)
	hi, lo = bits.Mul64(u0, R>>4)
	c_lo, carry = bits.Add64(c_lo, lo, 0)
	c_hi, _ = bits.Add64(c_hi, hi, carry)

	// rr[0] = (uint64_t)c & M
	r.n[0] = c_lo & M

	// c >>= 52
	c_lo = (c_lo >> 52) | (c_hi << 12)
	c_hi >>= 52

	// c += (uint128_t)a0 * b[1]
	hi, lo = bits.Mul64(a0, b1)
	c_lo, carry = bits.Add64(c_lo, lo, 0)
	c_hi, _ = bits.Add64(c_hi, hi, carry)

	// c += (uint128_t)a1 * b[0]
	hi, lo = bits.Mul64(a1, b0)
	c_lo, carry = bits.Add64(c_lo, lo, 0)
	c_hi, _ = bits.Add64(c_hi, hi, carry)

	// d += (uint128_t)a2 * b[4]
	hi, lo = bits.Mul64(a2, b4)
	d_lo, carry = bits.Add64(d_lo, lo, 0)
	d_hi, _ = bits.Add64(d_hi, hi, carry)

	// d += (uint128_t)a3 * b[3]
	hi, lo = bits.Mul64(a3, b3)
	d_lo, carry = bits.Add64(d_lo, lo, 0)
	d_hi, _ = bits.Add64(d_hi, hi, carry)

	// d += (uint128_t)a4 * b[2]
	hi, lo = bits.Mul64(a4, b2)
	d_lo, carry = bits.Add64(d_lo, lo, 0)
	d_hi, _ = bits.Add64(d_hi, hi, carry)

	// c += (uint128_t)((uint64_t)d & M) * R
	hi, lo = bits.Mul64(d_lo&M, R)
	c_lo, carry = bits.Add64(c_lo, lo, 0)
	c_hi, _ = bits.Add64(c_hi, hi, carry)

	// d >>= 52
	d_lo = (d_lo >> 52) | (d_hi << 12)
	d_hi >>= 52

	// rr[1] = (uint64_t)c & M
	r.n[1] = c_lo & M

	// c >>= 52
	c_lo = (c_lo >> 52) | (c_hi << 12)
	c_hi >>= 52

	// c += (uint128_t)a0 * b[2]
	hi, lo = bits.Mul64(a0, b2)
	c_lo, carry = bits.Add64(c_lo, lo, 0)
	c_hi, _ = bits.Add64(c_hi, hi, carry)

	// c += (uint128_t)a1 * b[1]
	hi, lo = bits.Mul64(a1, b1)
	c_lo, carry = bits.Add64(c_lo, lo, 0)
	c_hi, _ = bits.Add64(c_hi, hi, carry)

	// c += (uint128_t)a2 * b[0]
	hi, lo = bits.Mul64(a2, b0)
	c_lo, carry = bits.Add64(c_lo, lo, 0)
	c_hi, _ = bits.Add64(c_hi, hi, carry)

	// d += (uint128_t)a3 * b[4]
	hi, lo = bits.Mul64(a3, b4)
	d_lo, carry = bits.Add64(d_lo, lo, 0)
	d_hi, _ = bits.Add64(d_hi, hi, carry)

	// d += (uint128_t)a4 * b[3]
	hi, lo = bits.Mul64(a4, b3)
	d_lo, carry = bits.Add64(d_lo, lo, 0)
	d_hi, _ = bits.Add64(d_hi, hi, carry)

	// c += (uint128_t)R * (uint64_t)d
	hi, lo = bits.Mul64(R, d_lo)
	c_lo, carry = bits.Add64(c_lo, lo, 0)
	c_hi, _ = bits.Add64(c_hi, hi, carry)

	// d >>= 64
	d_lo = d_hi
	d_hi = 0

	// rr[2] = (uint64_t)c & M
	r.n[2] = c_lo & M

	// c >>= 52
	c_lo = (c_lo >> 52) | (c_hi << 12)
	c_hi >>= 52

	// c += (uint128_t)(R << 12) * (uint64_t)d
	hi, lo = bits.Mul64(R<<12, d_lo)
	c_lo, carry = bits.Add64(c_lo, lo, 0)
	c_hi, _ = bits.Add64(c_hi, hi, carry)

	// c += t3
	c_lo, carry = bits.Add64(c_lo, t3, 0)
	c_hi, _ = bits.Add64(c_hi, 0, carry)

	// rr[3] = (uint64_t)c & M
	r.n[3] = c_lo & M

	// c >>= 52
	c_lo = (c_lo >> 52) | (c_hi << 12)
	c_hi >>= 52

	// rr[4] = (uint64_t)c + t4
	r.n[4] = c_lo + t4
}

func (a *Field) Sqr(r *Field) {
	a0, a1, a2, a3, a4 := a.n[0], a.n[1], a.n[2], a.n[3], a.n[4]

	var c_lo, c_hi, d_lo, d_hi uint64
	var t3, t4, tx, u0 uint64
	var hi, lo, carry uint64

	// d = (a0*2) * a3
	d_hi, d_lo = bits.Mul64(a0*2, a3)

	// d += (a1*2) * a2
	hi, lo = bits.Mul64(a1*2, a2)
	d_lo, carry = bits.Add64(d_lo, lo, 0)
	d_hi, _ = bits.Add64(d_hi, hi, carry)

	// c = a4 * a4
	c_hi, c_lo = bits.Mul64(a4, a4)

	// d += (uint64_t)c * R
	hi, lo = bits.Mul64(c_lo, R)
	d_lo, carry = bits.Add64(d_lo, lo, 0)
	d_hi, _ = bits.Add64(d_hi, hi, carry)

	// c >>= 64
	// After shift, c contains only the high 64 bits of the original c

	// t3 = (uint64_t)d & M
	t3 = d_lo & M

	// d >>= 52
	d_lo = (d_lo >> 52) | (d_hi << 12)
	d_hi >>= 52

	// a4 *= 2
	a4 *= 2

	// d += (uint128_t)a0 * a4
	hi, lo = bits.Mul64(a0, a4)
	d_lo, carry = bits.Add64(d_lo, lo, 0)
	d_hi, _ = bits.Add64(d_hi, hi, carry)

	// d += (uint128_t)(a1*2) * a3
	hi, lo = bits.Mul64(a1*2, a3)
	d_lo, carry = bits.Add64(d_lo, lo, 0)
	d_hi, _ = bits.Add64(d_hi, hi, carry)

	// d += (uint128_t)a2 * a2
	hi, lo = bits.Mul64(a2, a2)
	d_lo, carry = bits.Add64(d_lo, lo, 0)
	d_hi, _ = bits.Add64(d_hi, hi, carry)

	// d += (uint128_t)(R << 12) * (uint64_t)c
	hi, lo = bits.Mul64(R<<12, c_hi)
	d_lo, carry = bits.Add64(d_lo, lo, 0)
	d_hi, _ = bits.Add64(d_hi, hi, carry)

	// t4 = (uint64_t)d & M
	t4 = d_lo & M

	// d >>= 52
	d_lo = (d_lo >> 52) | (d_hi << 12)
	d_hi >>= 52

	// tx = (t4 >> 48)
	tx = t4 >> 48

	// t4 &= (M >> 4)
	t4 &= (M >> 4)

	// c = (uint128_t)a0 * a0
	c_hi, c_lo = bits.Mul64(a0, a0)

	// d += (uint128_t)a1 * a4
	hi, lo = bits.Mul64(a1, a4)
	d_lo, carry = bits.Add64(d_lo, lo, 0)
	d_hi, _ = bits.Add64(d_hi, hi, carry)

	// d += (uint128_t)(a2*2) * a3
	hi, lo = bits.Mul64(a2*2, a3)
	d_lo, carry = bits.Add64(d_lo, lo, 0)
	d_hi, _ = bits.Add64(d_hi, hi, carry)

	// u0 = (uint64_t)d & M
	u0 = d_lo & M

	// d >>= 52
	d_lo = (d_lo >> 52) | (d_hi << 12)
	d_hi >>= 52

	// u0 = (u0 << 4) | tx
	u0 = (u0 << 4) | tx

	// c += (uint128_t)u0 * (R >> 4)
	hi, lo = bits.Mul64(u0, R>>4)
	c_lo, carry = bits.Add64(c_lo, lo, 0)
	c_hi, _ = bits.Add64(c_hi, hi, carry)

	// rr[0] = (uint64_t)c & M
	r.n[0] = c_lo & M

	// c >>= 52
	c_lo = (c_lo >> 52) | (c_hi << 12)
	c_hi >>= 52

	// a0 *= 2
	a0 *= 2

	// c += (uint128_t)a0 * a1
	hi, lo = bits.Mul64(a0, a1)
	c_lo, carry = bits.Add64(c_lo, lo, 0)
	c_hi, _ = bits.Add64(c_hi, hi, carry)

	// d += (uint128_t)a2 * a4
	hi, lo = bits.Mul64(a2, a4)
	d_lo, carry = bits.Add64(d_lo, lo, 0)
	d_hi, _ = bits.Add64(d_hi, hi, carry)

	// d += (uint128_t)a3 * a3
	hi, lo = bits.Mul64(a3, a3)
	d_lo, carry = bits.Add64(d_lo, lo, 0)
	d_hi, _ = bits.Add64(d_hi, hi, carry)

	// c += (uint128_t)((uint64_t)d & M) * R
	hi, lo = bits.Mul64(d_lo&M, R)
	c_lo, carry = bits.Add64(c_lo, lo, 0)
	c_hi, _ = bits.Add64(c_hi, hi, carry)

	// d >>= 52
	d_lo = (d_lo >> 52) | (d_hi << 12)
	d_hi >>= 52

	// rr[1] = (uint64_t)c & M
	r.n[1] = c_lo & M

	// c >>= 52
	c_lo = (c_lo >> 52) | (c_hi << 12)
	c_hi >>= 52

	// c += (uint128_t)a0 * a2
	hi, lo = bits.Mul64(a0, a2)
	c_lo, carry = bits.Add64(c_lo, lo, 0)
	c_hi, _ = bits.Add64(c_hi, hi, carry)

	// c += (uint128_t)a1 * a1
	hi, lo = bits.Mul64(a1, a1)
	c_lo, carry = bits.Add64(c_lo, lo, 0)
	c_hi, _ = bits.Add64(c_hi, hi, carry)

	// d += (uint128_t)a3 * a4
	hi, lo = bits.Mul64(a3, a4)
	d_lo, carry = bits.Add64(d_lo, lo, 0)
	d_hi, _ = bits.Add64(d_hi, hi, carry)

	// c += (uint128_t)R * (uint64_t)d
	// This multiplies R by the low 64 bits of d and adds to c
	hi, lo = bits.Mul64(R, d_lo)
	c_lo, carry = bits.Add64(c_lo, lo, 0)
	c_hi, _ = bits.Add64(c_hi, hi, carry)

	// d >>= 64
	// After this, d contains only what was in d_hi
	d_lo = d_hi
	d_hi = 0

	// rr[2] = (uint64_t)c & M
	r.n[2] = c_lo & M

	// c >>= 52
	c_lo = (c_lo >> 52) | (c_hi << 12)
	c_hi >>= 52

	// c += (uint128_t)(R << 12) * (uint64_t)d
	hi, lo = bits.Mul64(R<<12, d_lo)
	c_lo, carry = bits.Add64(c_lo, lo, 0)
	c_hi, _ = bits.Add64(c_hi, hi, carry)

	// c += t3
	c_lo, carry = bits.Add64(c_lo, t3, 0)
	c_hi, _ = bits.Add64(c_hi, 0, carry)

	// rr[3] = (uint64_t)c & M
	r.n[3] = c_lo & M

	// c >>= 52
	c_lo = (c_lo >> 52) | (c_hi << 12)
	c_hi >>= 52

	// rr[4] = (uint64_t)c + t4
	r.n[4] = c_lo + t4
}
