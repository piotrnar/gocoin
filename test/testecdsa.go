package main

import (
	"math/big"
	"encoding/hex"
	"piotr/btc"
	"crypto/ecdsa"
)

const (
	pubX = "0eaebcd1df2df853d66ce0e1b0fda07f67d1cabefde98514aad795b86a6ea66d"
	pubY = "beb26b67d7a00e2447baeccc8a4cef7cd3cad67376ac1c5785aeebb4f6441c16"
	sigR = "00fe00e013c244062847045ae7eb73b03fca583e9aa5dbd030a8fd1c6dfcf11b10"
	sigS = "7d0d04fed8fa1e93007468d5a9e134b0a7023b6d31db4e50942d43a250f4d07c"
	msgRaw = "01000000014d276db8e3a547cc3eaff4051d0d158da21724634d7c67c51129fa403dded5de010000001976a914718950ac3039e53fbd6eb0213de333b689a1ca1288acffffffff02a8d39b0f000000001976a914db641fc6dff262fe2504725f2c4c1852b18ffe3588ace693f205000000001976a9141321c4f37c5b2be510c1c7725a83e561ad27876b88ac00000000"
	msgH = "3382219555ddbb5b00e0090f469e590ba1eae03c7f28ab937de330aa60294ed6"
)


func bigIntFromHex(s string) *big.Int {
	r, ok := new(big.Int).SetString(s, 16)
	if !ok {
		panic("bad hex")
	}
	return r
}

func main () {
	pub := ecdsa.PublicKey{
		Curve: btc.S256(),
		X:     bigIntFromHex(pubX),
		Y:     bigIntFromHex(pubY),
	}
	println("x", pub.X.String())
	println("y", pub.Y.String())
	
	msg, er := hex.DecodeString(msgRaw + "01000000")
	if er != nil {
		panic(er.Error())
	}

	h := btc.NewSha2Hash(msg[:])
	println(h.String())
	
	r := bigIntFromHex(sigR)
	s := bigIntFromHex(sigS)
	println("r", r.String())
	println("s", s.String())
	ok := ecdsa.Verify(&pub, h.Hash[:], r, s)
	println(ok)
}
