package secp256k1

func (a *Fe_t) sqr(r *Fe_t) {
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
