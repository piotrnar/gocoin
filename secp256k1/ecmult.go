package secp256k1

import (
//	"fmt"
)


var (
	pre_g, pre_g_128 []XY
	prec [64][16]XY
	fin XY
)


func ecmult_start() {
	g := TheCurve.G

	// calculate 2^128*generator
	var g_128j XYZ
	g_128j.set_ge(&g)

	for i := 0; i < 128; i++ {
		g_128j.Double(&g_128j)
	}

	var g_128 XY
	g_128.set_gej(&g_128j)

    // precompute the tables with odd multiples
	pre_g = g.precomp(WINDOW_G)
	pre_g_128 = g_128.precomp(WINDOW_G)

	// compute prec and fin
	var gg XYZ
	gg.set_ge(&g)
	ad := g
	var fn XYZ
	fn.Infinity = true
	for j:=0; j<64; j++ {
		prec[j][0].set_gej(&gg)
		fn.Add(&fn, &gg)
		for i:=1; i<16; i++ {
			gg.AddXY(&gg, &ad)
			prec[j][i].set_gej(&gg)
		}
		ad = prec[j][15]
	}
	fin.set_gej(&fn)
	fin.Neg(&fin)
}


func ecmult_wnaf(wnaf []int, a *Number, w uint) (ret int) {
	var zeroes uint
	var X Number
	X.Set(&a.Int)

	for X.Sign()!=0 {
		for X.Bit(0)==0 {
			zeroes++
			X.rsh(1)
		}
		word := X.rsh_x(w)
		for zeroes > 0 {
			wnaf[ret] = 0
			ret++
			zeroes--
		}
		if (word & (1 << (w-1))) != 0 {
			X.inc()
			wnaf[ret] = (word - (1 << w))
		} else {
			wnaf[ret] = word
		}
		zeroes = w-1
		ret++
	}
	return
}

func ecmult_gen(r *XYZ, gn *Number) {
	var n Number;
	n.Set(&gn.Int)
	r.set_ge(&prec[0][n.rsh_x(4)])
	for j:=1; j<64; j++ {
		r.AddXY(r, &prec[j][n.rsh_x(4)])
	}
	r.AddXY(r, &fin)
}
