package newec

import (
//	"fmt"
)


var (
	pre_g, pre_g_128 []ge_t
	prec [64][16]ge_t
	fin ge_t
)


func ecmult_start() {
	g := secp256k1.g

	// calculate 2^128*generator
	var g_128j gej_t
	g_128j.set_ge(&g)

	for i := 0; i < 128; i++ {
		g_128j.double(&g_128j)
	}

	var g_128 ge_t
	g_128.set_gej(&g_128j)

    // precompute the tables with odd multiples
	pre_g = g.precomp(WINDOW_G)
	pre_g_128 = g_128.precomp(WINDOW_G)

	// compute prec and fin
	var gg gej_t
	gg.set_ge(&g)
	ad := g
	var fn gej_t
	fn.infinity = true
	for j:=0; j<64; j++ {
		prec[j][0].set_gej(&gg)
		fn.add(&fn, &gg)
		for i:=1; i<16; i++ {
			gg.add_ge(&gg, &ad)
			prec[j][i].set_gej(&gg)
		}
		ad = prec[j][15]
	}
	fin.set_gej(&fn)
	fin.neg(&fin)
}


func ecmult_wnaf(wnaf []int, a *Number, w uint) (ret int) {
	var zeroes uint
	var x Number
	x.Set(&a.Int)

	for x.Sign()!=0 {
		for x.Bit(0)==0 {
			zeroes++
			x.rsh(1)
		}
		word := x.rsh_x(w)
		for zeroes > 0 {
			wnaf[ret] = 0
			ret++
			zeroes--
		}
		if (word & (1 << (w-1))) != 0 {
			x.inc()
			wnaf[ret] = (word - (1 << w))
		} else {
			wnaf[ret] = word
		}
		zeroes = w-1
		ret++
	}
	return
}

func ecmult_gen(r *gej_t, gn *Number) {
	var n Number;
	n.Set(&gn.Int)
	r.set_ge(&prec[0][n.rsh_x(4)])
	for j:=1; j<64; j++ {
		r.add_ge(r, &prec[j][n.rsh_x(4)])
	}
	r.add_ge(r, &fin)
}
