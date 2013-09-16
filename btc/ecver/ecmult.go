package ecver

import (
)

var (
	pre_g, pre_g_128 []secp256k1_ge_t
	prec [64][16]secp256k1_ge_t
	fin secp256k1_ge_t
)


func secp256k1_ecmult_start() {
	var g secp256k1_ge_t
	g.x.Set(secp256k1.Gx)
	g.y.Set(secp256k1.Gy)

	// calculate 2^128*generator
	var g_128j secp256k1_gej_t
	g_128j.set_ge(&g)

	for i := 0; i < 128; i++ {
		g_128j.double_p(&g_128j)
	}

	var g_128 secp256k1_ge_t
	g_128.set_gej(&g_128j)

    // precompute the tables with odd multiples
	pre_g = g.precomp(WINDOW_G)
	pre_g_128 = g_128.precomp(WINDOW_G)

	// compute prec and fin
	var gg secp256k1_gej_t
	gg.set_ge(&g)
	ad := g
	var fn secp256k1_gej_t
	fn.infinity = true
	for j:=0; j<64; j++ {
		prec[j][0].set_gej(&gg)
		fn.add_p(&fn, &gg)
		for i:=1; i<16; i++ {
			gg.add_ge_p(&gg, &ad)
			prec[j][i].set_gej(&gg)
		}
		ad = prec[j][15]
	}
	fin.set_gej(&fn)
	fin.neg_p(&fin)
}


func secp256k1_ecmult_wnaf(wnaf []int, a *secp256k1_num_t, w uint) (ret int) {
	var zeroes uint

	x := new_num_val(a)

	for x.Sign()!=0 {
		for x.Bit(0)==0 {
			zeroes++
			x.Rsh(&x.Int, 1)
		}
		word := x.shift(w)
		for zeroes > 0 {
			wnaf[ret] = 0
			ret++
			zeroes--
		}
		if (word & (1 << (w-1))) != 0 {
			x.Add(&x.Int, BigInt1)
			wnaf[ret] = (word - (1 << w))
		} else {
			wnaf[ret] = word
		}
		zeroes = w-1
		ret++
	}
	return
}

var dbg bool

func secp256k1_ecmult(a *secp256k1_gej_t, na, ng *secp256k1_num_t) (r *secp256k1_gej_t) {
	r = new(secp256k1_gej_t)

    // split na into na_1 and na_lam (where na = na_1 + na_lam*lambda, and na_1 and na_lam are ~128 bit)
	na_1, na_lam := na.split_exp()

	// split ng into ng_1 and ng_128 (where gn = gn_1 + gn_128*2^128, and gn_1 and gn_128 are ~128 bit)
	ng_1, ng_128 := ng.split(128)

	// build wnaf representation for na_1, na_lam, ng_1, ng_128
	var wnaf_na_1, wnaf_na_lam, wnaf_ng_1, wnaf_ng_128 [129]int
	bits_na_1 := secp256k1_ecmult_wnaf(wnaf_na_1[:], na_1, WINDOW_A)
	bits_na_lam := secp256k1_ecmult_wnaf(wnaf_na_lam[:], na_lam, WINDOW_A)
	bits_ng_1 := secp256k1_ecmult_wnaf(wnaf_ng_1[:], ng_1, WINDOW_G)

	bits_ng_128 := secp256k1_ecmult_wnaf(wnaf_ng_128[:], ng_128, WINDOW_G)

	// calculate a_lam = a*lambda
	a_lam := *a
	a_lam.mul_lambda_s()

	// calculate odd multiples of a and a_lam
	pre_a_1 := a.precomp(WINDOW_A)
	pre_a_lam := a_lam.precomp(WINDOW_A)

	bits := bits_na_1
	if bits_na_lam > bits {
		bits = bits_na_lam
	}
	if bits_ng_1 > bits {
		bits = bits_ng_1
	}
	if bits_ng_128 > bits {
		bits = bits_ng_128
	}

	r.infinity = true

	var tmpj secp256k1_gej_t
	var tmpa secp256k1_ge_t
	var n int

	for i:=bits-1; i>=0; i-- {
		r.double_p(r)

		if i < bits_na_1 {
			n = wnaf_na_1[i]
			if n!=0 {
				ECMULT_TABLE_GET_GEJ(&tmpj, pre_a_1, n, WINDOW_A)
				r.add_p(r, &tmpj)
			}
		}

		if i < bits_na_lam {
			n = wnaf_na_lam[i]
			if n!=0 {
				ECMULT_TABLE_GET_GEJ(&tmpj, pre_a_lam, n, WINDOW_A)
				r.add_p(r, &tmpj)
			}
		}

		if i < bits_ng_1 {
			n = wnaf_ng_1[i]
			if n!=0 {
				ECMULT_TABLE_GET_GE(&tmpa, pre_g, n, WINDOW_G)
				r.add_ge_p(r, &tmpa)
			}
		}

		if i < bits_ng_128 {
			n = wnaf_ng_128[i]
			if n!=0 {
				ECMULT_TABLE_GET_GE(&tmpa, pre_g_128, n, WINDOW_G);
				r.add_ge_p(r, &tmpa)
			}
		}
	}

	return
}


func ECMULT_TABLE_GET_GEJ(r *secp256k1_gej_t, pre []secp256k1_gej_t, n int, w uint) {
	if n > 0 {
		*r = pre[((n)-1)/2]
	} else {
		pre[(-(n)-1)/2].neg_p(r)
	}
}

func ECMULT_TABLE_GET_GE(r *secp256k1_ge_t, pre []secp256k1_ge_t, n int, w uint) {
	if n > 0 {
		*r = pre[((n)-1)/2]
	} else {
		pre[(-(n)-1)/2].neg_p(r)
	}
}
