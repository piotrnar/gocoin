package main

import (
	"math/big"
	"encoding/hex"
	"github.com/piotrnar/gocoin/btc"
)

var curv *btc.BitCurve = btc.S256()

func main() {
	//secret, _ := new(big.Int).SetString("f8f32e723decf4051aefac8e2c93c9c5b214313817cdb01a1494b917c8436b35", 16)
	secret, _ := new(big.Int).SetString("9242353464575846756867969577867969568679679780707897896856436b35", 16)

	private_key, _ := new(big.Int).SetString("e8f32e723decf4051aefac8e2c93c9c5b214313817cdb01a1494b917c8436b35", 16)

	println("s", hex.EncodeToString(private_key.Bytes()))
	println("q", hex.EncodeToString(secret.Bytes()))

	x, y := curv.ScalarBaseMult(private_key.Bytes())
	println("x", hex.EncodeToString(x.Bytes()))
	println("y", hex.EncodeToString(y.Bytes()))

	for i:=0; i<10; i++ {
		println(i)
		//B_private_key = A_private_key + B_secret
		private_key_B := new(big.Int).Add(private_key, secret)
		println("sb", hex.EncodeToString(private_key_B.Bytes()))

		private_key_B = new(big.Int).Mod(private_key_B, curv.N)
		println("sb", hex.EncodeToString(private_key_B.Bytes()))

		xB, yB := curv.ScalarBaseMult(private_key_B.Bytes())
		println("xb", hex.EncodeToString(xB.Bytes()))
		println("yb", hex.EncodeToString(yB.Bytes()))

		//B_public_key = B_secret*point + A_public_key
		bspX, bspY := curv.ScalarBaseMult(secret.Bytes())
		bX, bY := curv.Add(x, y, bspX, bspY)
		println("x-", hex.EncodeToString(bX.Bytes()))
		println("yi", hex.EncodeToString(bY.Bytes()))

		private_key = private_key_B
		x, y = bX, bY
	}
}
