// +build amd64 arm64 arm64be ppc64 ppc64le mips64 mips64le s390x sparc64

package secp256k1

import (
	"math/bits"
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
	var c_lo, c_hi, d_lo, d_hi uint64
	var t3, t4, tx, u0, rn0, rn1 uint64
	var carry, him, lom uint64

	a0 := a.n[0]
	a1 := a.n[1]
	a2 := a.n[2]
	a3 := a.n[3]
	a4 := a.n[4]

	const M = 0xFFFFFFFFFFFFF
	const R = 0x1000003D10

	/*  [... a b c] is a shorthand for ... + a<<104 + b<<52 + c<<0 mod n.
	 *  for 0 <= x <= 4, px is a shorthand for sum(a[i]*b[x-i], i=0..x).
	 *  for 4 <= x <= 8, px is a shorthand for sum(a[i]*b[x-i], i=(x-4)..4)
	 *  Note that [x 0 0 0 0 0] = [x*R].
	 */

	//d.AddMul64s(a0, b.n[3])
	d_hi, d_lo = bits.Mul64(a0, b.n[3])

	//d.AddMul64s(a1, b.n[2])
	him, lom = bits.Mul64(a1, b.n[2])
	d_lo, carry = bits.Add64(d_lo, lom, 0)
	d_hi, _ = bits.Add64(d_hi, him, carry)

	//d.AddMul64s(a2, b.n[1])
	him, lom = bits.Mul64(a2, b.n[1])
	d_lo, carry = bits.Add64(d_lo, lom, 0)
	d_hi, _ = bits.Add64(d_hi, him, carry)

	//d.AddMul64s(a3, b.n[0])
	him, lom = bits.Mul64(a3, b.n[0])
	d_lo, carry = bits.Add64(d_lo, lom, 0)
	d_hi, _ = bits.Add64(d_hi, him, carry)

	/* [d 0 0 0] = [p3 0 0 0] */
	//c = From64(a4).Mul64(b.n[4])
	c_hi, c_lo = bits.Mul64(a4, b.n[4])

	/* [c 0 0 0 0 d 0 0 0] = [p8 0 0 0 0 p3 0 0 0] */
	//d = d.Add(c.And64(M).Mul64(R))
	him, lom = bits.Mul64(c_lo&M, R)
	d_lo, carry = bits.Add64(d_lo, lom, 0)
	d_hi, _ = bits.Add64(d_hi, him, carry)

	//c = c.Rsh52()
	c_lo = c_lo>>52 | c_hi<<(64-52)
	c_hi = c_hi >> 52

	/* [c 0 0 0 0 0 d 0 0 0] = [p8 0 0 0 0 p3 0 0 0] */
	t3 = d_lo & M
	//d = d.Rsh52()
	d_lo = d_lo>>52 | d_hi<<(64-52)
	d_hi = d_hi >> 52
	/* [c 0 0 0 0 d t3 0 0 0] = [p8 0 0 0 0 p3 0 0 0] */

	//d = d.Add(From64(a0).Mul64(b.n[4]))
	him, lom = bits.Mul64(a0, b.n[4])
	d_lo, carry = bits.Add64(d_lo, lom, 0)
	d_hi, _ = bits.Add64(d_hi, him, carry)

	//d = d.Add(From64(a1).Mul64(b.n[3]))
	him, lom = bits.Mul64(a1, b.n[3])
	d_lo, carry = bits.Add64(d_lo, lom, 0)
	d_hi, _ = bits.Add64(d_hi, him, carry)

	//d = d.Add(From64(a2).Mul64(b.n[2]))
	him, lom = bits.Mul64(a2, b.n[2])
	d_lo, carry = bits.Add64(d_lo, lom, 0)
	d_hi, _ = bits.Add64(d_hi, him, carry)

	//d = d.Add(From64(a3).Mul64(b.n[1]))
	him, lom = bits.Mul64(a3, b.n[1])
	d_lo, carry = bits.Add64(d_lo, lom, 0)
	d_hi, _ = bits.Add64(d_hi, him, carry)

	//d = d.Add(From64(a4).Mul64(b.n[0]))
	him, lom = bits.Mul64(a4, b.n[0])
	d_lo, carry = bits.Add64(d_lo, lom, 0)
	d_hi, _ = bits.Add64(d_hi, him, carry)

	//d = d.Add(c.Mul64(R))
	him, lom = bits.Mul64(c_lo, R)
	him += c_hi * R
	d_lo, carry = bits.Add64(d_lo, lom, 0)
	d_hi, _ = bits.Add64(d_hi, him, carry)

	/* [d t3 0 0 0] = [p8 0 0 0 p4 p3 0 0 0] */
	t4 = d_lo & M

	//d = d.Rsh52()
	d_lo = d_lo>>52 | d_hi<<(64-52)
	d_hi = d_hi >> 52

	/* [d t4 t3 0 0 0] = [p8 0 0 0 p4 p3 0 0 0] */
	tx = (t4 >> 48)
	t4 &= (M >> 4)
	/* [d t4+(tx<<48) t3 0 0 0] = [p8 0 0 0 p4 p3 0 0 0] */

	//c = From64(a0).Mul64(b.n[0])
	c_hi, c_lo = bits.Mul64(a0, b.n[0])

	/* [d t4+(tx<<48) t3 0 0 c] = [p8 0 0 0 p4 p3 0 0 p0] */
	//d = d.Add(From64(a1).Mul64(b.n[4]))
	him, lom = bits.Mul64(a1, b.n[4])
	d_lo, carry = bits.Add64(d_lo, lom, 0)
	d_hi, _ = bits.Add64(d_hi, him, carry)

	//d = d.Add(From64(a2).Mul64(b.n[3]))
	him, lom = bits.Mul64(a2, b.n[3])
	d_lo, carry = bits.Add64(d_lo, lom, 0)
	d_hi, _ = bits.Add64(d_hi, him, carry)

	//d = d.Add(From64(a3).Mul64(b.n[2]))
	him, lom = bits.Mul64(a3, b.n[2])
	d_lo, carry = bits.Add64(d_lo, lom, 0)
	d_hi, _ = bits.Add64(d_hi, him, carry)

	//d = d.Add(From64(a4).Mul64(b.n[1]))
	him, lom = bits.Mul64(a4, b.n[1])
	d_lo, carry = bits.Add64(d_lo, lom, 0)
	d_hi, _ = bits.Add64(d_hi, him, carry)

	/* [d t4+(tx<<48) t3 0 0 c] = [p8 0 0 p5 p4 p3 0 0 p0] */
	u0 = d_lo & M

	//d = d.Rsh52()
	d_lo = d_lo>>52 | d_hi<<(64-52)
	d_hi = d_hi >> 52

	/* [d u0 t4+(tx<<48) t3 0 0 c] = [p8 0 0 p5 p4 p3 0 0 p0] */
	/* [d 0 t4+(tx<<48)+(u0<<52) t3 0 0 c] = [p8 0 0 p5 p4 p3 0 0 p0] */
	u0 = (u0 << 4) | tx
	/* [d 0 t4+(u0<<48) t3 0 0 c] = [p8 0 0 p5 p4 p3 0 0 p0] */

	//c = c.Add(From64(u0).Mul64(R >> 4))
	him, lom = bits.Mul64(u0, R>>4)
	c_lo, carry = bits.Add64(c_lo, lom, 0)
	c_hi, _ = bits.Add64(c_hi, him, carry)

	/* [d 0 t4 t3 0 0 c] = [p8 0 0 p5 p4 p3 0 0 p0] */
	rn0 = c_lo & M
	//c = c.Rsh52()
	c_lo = c_lo>>52 | c_hi<<(64-52)
	c_hi = c_hi >> 52

	/* [d 0 t4 t3 0 c r0] = [p8 0 0 p5 p4 p3 0 0 p0] */

	//c = c.Add(From64(a0).Mul64(b.n[1]))
	him, lom = bits.Mul64(a0, b.n[1])
	c_lo, carry = bits.Add64(c_lo, lom, 0)
	c_hi, _ = bits.Add64(c_hi, him, carry)

	//c = c.Add(From64(a1).Mul64(b.n[0]))
	him, lom = bits.Mul64(a1, b.n[0])
	c_lo, carry = bits.Add64(c_lo, lom, 0)
	c_hi, _ = bits.Add64(c_hi, him, carry)

	/* [d 0 t4 t3 0 c r0] = [p8 0 0 p5 p4 p3 0 p1 p0] */
	//d = d.Add(From64(a2).Mul64(b.n[4]))
	him, lom = bits.Mul64(a2, b.n[4])
	d_lo, carry = bits.Add64(d_lo, lom, 0)
	d_hi, _ = bits.Add64(d_hi, him, carry)

	//d = d.Add(From64(a3).Mul64(b.n[3]))
	him, lom = bits.Mul64(a3, b.n[3])
	d_lo, carry = bits.Add64(d_lo, lom, 0)
	d_hi, _ = bits.Add64(d_hi, him, carry)

	//d = d.Add(From64(a4).Mul64(b.n[2]))
	him, lom = bits.Mul64(a4, b.n[2])
	d_lo, carry = bits.Add64(d_lo, lom, 0)
	d_hi, _ = bits.Add64(d_hi, him, carry)

	/* [d 0 t4 t3 0 c r0] = [p8 0 p6 p5 p4 p3 0 p1 p0] */
	//c = c.Add(From64(d_lo & M).Mul64(R))
	him, lom = bits.Mul64(d_lo&M, R)
	c_lo, carry = bits.Add64(c_lo, lom, 0)
	c_hi, _ = bits.Add64(c_hi, him, carry)

	//d = d.Rsh52()
	d_lo = d_lo>>52 | d_hi<<(64-52)
	d_hi = d_hi >> 52

	/* [d 0 0 t4 t3 0 c r0] = [p8 0 p6 p5 p4 p3 0 p1 p0] */
	rn1 = c_lo & M
	//c = c.Rsh52()
	c_lo = c_lo>>52 | c_hi<<(64-52)
	c_hi = c_hi >> 52

	/* [d 0 0 t4 t3 c r1 r0] = [p8 0 p6 p5 p4 p3 0 p1 p0] */

	//c = c.Add(From64(a0).Mul64(b.n[2]))
	him, lom = bits.Mul64(a0, b.n[2])
	c_lo, carry = bits.Add64(c_lo, lom, 0)
	c_hi, _ = bits.Add64(c_hi, him, carry)

	//c = c.Add(From64(a1).Mul64(b.n[1]))
	him, lom = bits.Mul64(a1, b.n[1])
	c_lo, carry = bits.Add64(c_lo, lom, 0)
	c_hi, _ = bits.Add64(c_hi, him, carry)

	//c = c.Add(From64(a2).Mul64(b.n[0]))
	him, lom = bits.Mul64(a2, b.n[0])
	c_lo, carry = bits.Add64(c_lo, lom, 0)
	c_hi, _ = bits.Add64(c_hi, him, carry)

	/* [d 0 0 t4 t3 c r1 r0] = [p8 0 p6 p5 p4 p3 p2 p1 p0] */
	//d = d.Add(From64(a3).Mul64(b.n[4]))
	him, lom = bits.Mul64(a3, b.n[4])
	d_lo, carry = bits.Add64(d_lo, lom, 0)
	d_hi, _ = bits.Add64(d_hi, him, carry)

	//d = d.Add(From64(a4).Mul64(b.n[3]))
	him, lom = bits.Mul64(a4, b.n[3])
	d_lo, carry = bits.Add64(d_lo, lom, 0)
	d_hi, _ = bits.Add64(d_hi, him, carry)

	/* [d 0 0 t4 t3 c t1 r0] = [p8 p7 p6 p5 p4 p3 p2 p1 p0] */
	//c = c.Add(From64(d_lo & M).Mul64(R))
	him, lom = bits.Mul64(d_lo&M, R)
	c_lo, carry = bits.Add64(c_lo, lom, 0)
	c_hi, _ = bits.Add64(c_hi, him, carry)

	//d = d.Rsh52()
	d_lo = d_lo>>52 | d_hi<<(64-52)
	d_hi = d_hi >> 52

	/* [d 0 0 0 t4 t3 c r1 r0] = [p8 p7 p6 p5 p4 p3 p2 p1 p0] */

	r.n[0] = rn0
	r.n[1] = rn1

	/* [d 0 0 0 t4 t3 c r1 r0] = [p8 p7 p6 p5 p4 p3 p2 p1 p0] */
	r.n[2] = c_lo & M
	//c = c.Rsh52()
	c_lo = c_lo>>52 | c_hi<<(64-52)
	c_hi = c_hi >> 52
	/* [d 0 0 0 t4 t3+c r2 r1 r0] = [p8 p7 p6 p5 p4 p3 p2 p1 p0] */

	//c = c.Add(d.Mul64(R).Add64(t3))
	him, lom = bits.Mul64(d_lo, R)
	him += d_hi * R
	lom, carry = bits.Add64(lom, t3, 0)
	him += carry
	c_lo, carry = bits.Add64(c_lo, lom, 0)
	c_hi, _ = bits.Add64(c_hi, him, carry)

	/* [t4 c r2 r1 r0] = [p8 p7 p6 p5 p4 p3 p2 p1 p0] */
	r.n[3] = c_lo & M
	//c = c.Rsh52()

	r.n[4] = (c_lo>>52 | c_hi<<(64-52)) + t4
}

func (a *Field) Sqr(r *Field) {
	var c_lo, c_hi, d_lo, d_hi uint64
	var carry, him, lom uint64

	a0 := a.n[0]
	a1 := a.n[1]
	a2 := a.n[2]
	a3 := a.n[3]
	a4 := a.n[4]
	var t3, t4, tx, u0 uint64
	const (
		M = 0xFFFFFFFFFFFFF
		R = 0x1000003D10
	)

	/**  [... a b c] is a shorthand for ... + a<<104 + b<<52 + c<<0 mod n.
	 *  px is a shorthand for sum(a[i]*a[x-i], i=0..x).
	 *  Note that [x 0 0 0 0 0] = [x*R].
	 */

	//d = From64(a0 * 2).Mul64(a3)
	d_hi, d_lo = bits.Mul64(a0*2, a3)

	//d = d.Add(From64(a1 * 2).Mul64(a2))
	him, lom = bits.Mul64(a1*2, a2)
	d_lo, carry = bits.Add64(d_lo, lom, 0)
	d_hi, _ = bits.Add64(d_hi, him, carry)

	/* [d 0 0 0] = [p3 0 0 0] */
	//c = From64(a4).Mul64(a4)
	him, lom = bits.Mul64(a4, a4)
	c_lo, carry = bits.Add64(c_lo, lom, 0)
	c_hi, _ = bits.Add64(c_hi, him, carry)

	/* [c 0 0 0 0 d 0 0 0] = [p8 0 0 0 0 p3 0 0 0] */
	//d = d.Add(c.And64(M).Mul64(R))
	him, lom = bits.Mul64(c_lo&M, R)
	d_lo, carry = bits.Add64(d_lo, lom, 0)
	d_hi, _ = bits.Add64(d_hi, him, carry)

	//c = c.Rsh52()
	c_lo = c_lo>>52 | c_hi<<(64-52)
	c_hi = c_hi >> 52

	/* [c 0 0 0 0 0 d 0 0 0] = [p8 0 0 0 0 p3 0 0 0] */
	t3 = d_lo & M

	//d = d.Rsh52()
	d_lo = d_lo>>52 | d_hi<<(64-52)
	d_hi = d_hi >> 52

	/* [c 0 0 0 0 d t3 0 0 0] = [p8 0 0 0 0 p3 0 0 0] */

	a4 *= 2
	//d = d.Add(From64(a0).Mul64(a4))
	him, lom = bits.Mul64(a0, a4)
	d_lo, carry = bits.Add64(d_lo, lom, 0)
	d_hi, _ = bits.Add64(d_hi, him, carry)

	//d = d.Add(From64(a1 * 2).Mul64(a3))
	him, lom = bits.Mul64(a1*2, a3)
	d_lo, carry = bits.Add64(d_lo, lom, 0)
	d_hi, _ = bits.Add64(d_hi, him, carry)

	//d = d.Add(From64(a2).Mul64(a2))
	him, lom = bits.Mul64(a2, a2)
	d_lo, carry = bits.Add64(d_lo, lom, 0)
	d_hi, _ = bits.Add64(d_hi, him, carry)

	/* [c 0 0 0 0 d t3 0 0 0] = [p8 0 0 0 p4 p3 0 0 0] */

	//d = d.Add(c.Mul64(R))
	him, lom = bits.Mul64(c_lo, R)
	him += c_hi * R
	d_lo, carry = bits.Add64(d_lo, lom, 0)
	d_hi, _ = bits.Add64(d_hi, him, carry)

	/* [d t3 0 0 0] = [p8 0 0 0 p4 p3 0 0 0] */
	t4 = d_lo & M

	//d = d.Rsh52()
	d_lo = d_lo>>52 | d_hi<<(64-52)
	d_hi = d_hi >> 52

	/* [d t4 t3 0 0 0] = [p8 0 0 0 p4 p3 0 0 0] */
	tx = (t4 >> 48)
	t4 &= (M >> 4)
	/* [d t4+(tx<<48) t3 0 0 0] = [p8 0 0 0 p4 p3 0 0 0] */

	//c = From64(a0).Mul64(a0)
	c_hi, c_lo = bits.Mul64(a0, a0)

	/* [d t4+(tx<<48) t3 0 0 c] = [p8 0 0 0 p4 p3 0 0 p0] */
	//d = d.Add(From64(a1).Mul64(a4))
	him, lom = bits.Mul64(a1, a4)
	d_lo, carry = bits.Add64(d_lo, lom, 0)
	d_hi, _ = bits.Add64(d_hi, him, carry)

	//d = d.Add(From64(a2 * 2).Mul64(a3))
	him, lom = bits.Mul64(a2*2, a3)
	d_lo, carry = bits.Add64(d_lo, lom, 0)
	d_hi, _ = bits.Add64(d_hi, him, carry)

	/* [d t4+(tx<<48) t3 0 0 c] = [p8 0 0 p5 p4 p3 0 0 p0] */
	u0 = d_lo & M

	//d = d.Rsh52()
	d_lo = d_lo>>52 | d_hi<<(64-52)
	d_hi = d_hi >> 52

	/* [d u0 t4+(tx<<48) t3 0 0 c] = [p8 0 0 p5 p4 p3 0 0 p0] */
	/* [d 0 t4+(tx<<48)+(u0<<52) t3 0 0 c] = [p8 0 0 p5 p4 p3 0 0 p0] */
	u0 = (u0 << 4) | tx
	/* [d 0 t4+(u0<<48) t3 0 0 c] = [p8 0 0 p5 p4 p3 0 0 p0] */

	//c = c.Add(From64(u0).Mul64(R >> 4))
	him, lom = bits.Mul64(u0, R>>4)
	c_lo, carry = bits.Add64(c_lo, lom, 0)
	c_hi, _ = bits.Add64(c_hi, him, carry)

	/* [d 0 t4 t3 0 0 c] = [p8 0 0 p5 p4 p3 0 0 p0] */
	r.n[0] = c_lo & M

	//c = c.Rsh52()
	c_lo = c_lo>>52 | c_hi<<(64-52)
	c_hi = c_hi >> 52

	/* [d 0 t4 t3 0 c r0] = [p8 0 0 p5 p4 p3 0 0 p0] */

	a0 *= 2
	//c = c.Add(From64(a0).Mul64(a1))
	him, lom = bits.Mul64(a0, a1)
	c_lo, carry = bits.Add64(c_lo, lom, 0)
	c_hi, _ = bits.Add64(c_hi, him, carry)

	/* [d 0 t4 t3 0 c r0] = [p8 0 0 p5 p4 p3 0 p1 p0] */
	//d = d.Add(From64(a2).Mul64(a4))
	him, lom = bits.Mul64(a2, a4)
	d_lo, carry = bits.Add64(d_lo, lom, 0)
	d_hi, _ = bits.Add64(d_hi, him, carry)

	//d = d.Add(From64(a3).Mul64(a3))
	him, lom = bits.Mul64(a3, a3)
	d_lo, carry = bits.Add64(d_lo, lom, 0)
	d_hi, _ = bits.Add64(d_hi, him, carry)

	/* [d 0 t4 t3 0 c r0] = [p8 0 p6 p5 p4 p3 0 p1 p0] */
	//c = c.Add(From64(d_lo & M).Mul64(R))
	him, lom = bits.Mul64(d_lo&M, R)
	c_lo, carry = bits.Add64(c_lo, lom, 0)
	c_hi, _ = bits.Add64(c_hi, him, carry)

	//d = d.Rsh52()
	d_lo = d_lo>>52 | d_hi<<(64-52)
	d_hi = d_hi >> 52

	/* [d 0 0 t4 t3 0 c r0] = [p8 0 p6 p5 p4 p3 0 p1 p0] */
	r.n[1] = c_lo & M

	//c = c.Rsh52()
	c_lo = c_lo>>52 | c_hi<<(64-52)
	c_hi = c_hi >> 52
	/* [d 0 0 t4 t3 c r1 r0] = [p8 0 p6 p5 p4 p3 0 p1 p0] */

	//c = c.Add(From64(a0).Mul64(a2))
	him, lom = bits.Mul64(a0, a2)
	c_lo, carry = bits.Add64(c_lo, lom, 0)
	c_hi, _ = bits.Add64(c_hi, him, carry)

	//c = c.Add(From64(a1).Mul64(a1))
	him, lom = bits.Mul64(a1, a1)
	c_lo, carry = bits.Add64(c_lo, lom, 0)
	c_hi, _ = bits.Add64(c_hi, him, carry)

	/* [d 0 0 t4 t3 c r1 r0] = [p8 0 p6 p5 p4 p3 p2 p1 p0] */
	//d = d.Add(From64(a3).Mul64(a4))
	him, lom = bits.Mul64(a3, a4)
	d_lo, carry = bits.Add64(d_lo, lom, 0)
	d_hi, _ = bits.Add64(d_hi, him, carry)

	/* [d 0 0 t4 t3 c r1 r0] = [p8 p7 p6 p5 p4 p3 p2 p1 p0] */
	//c = c.Add(From64(d_lo & M).Mul64(R))
	him, lom = bits.Mul64(d_lo&M, R)
	c_lo, carry = bits.Add64(c_lo, lom, 0)
	c_hi, _ = bits.Add64(c_hi, him, carry)

	//d = d.Rsh52()
	d_lo = d_lo>>52 | d_hi<<(64-52)
	d_hi = d_hi >> 52

	/* [d 0 0 0 t4 t3 c r1 r0] = [p8 p7 p6 p5 p4 p3 p2 p1 p0] */
	r.n[2] = c_lo & M

	//c = c.Rsh52()
	c_lo = c_lo>>52 | c_hi<<(64-52)
	c_hi = c_hi >> 52

	/* [d 0 0 0 t4 t3+c r2 r1 r0] = [p8 p7 p6 p5 p4 p3 p2 p1 p0] */

	//c = c.Add(d.Mul64(R).Add64(t3))
	him, lom = bits.Mul64(d_lo, R)
	him += d_hi * R
	lom, carry = bits.Add64(lom, t3, 0)
	him += carry
	c_lo, carry = bits.Add64(c_lo, lom, 0)
	c_hi, _ = bits.Add64(c_hi, him, carry)

	/* [t4 c r2 r1 r0] = [p8 p7 p6 p5 p4 p3 p2 p1 p0] */
	r.n[3] = c_lo & M

	r.n[4] = (c_lo>>52 | c_hi<<(64-52)) + t4
}
