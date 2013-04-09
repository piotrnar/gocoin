package main

import (
	"encoding/hex"
	"github.com/piotrnar/gocoin/btc"
)

const (
	pubScr = "040eaebcd1df2df853d66ce0e1b0fda07f67d1cabefde98514aad795b86a6ea66dbeb26b67d7a00e2447baeccc8a4cef7cd3cad67376ac1c5785aeebb4f6441c16"
	sigScr = "3045022100fe00e013c244062847045ae7eb73b03fca583e9aa5dbd030a8fd1c6dfcf11b1002207d0d04fed8fa1e93007468d5a9e134b0a7023b6d31db4e50942d43a250f4d07c01"
	msgRaw = "01000000014d276db8e3a547cc3eaff4051d0d158da21724634d7c67c51129fa403dded5de010000001976a914718950ac3039e53fbd6eb0213de333b689a1ca1288acffffffff02a8d39b0f000000001976a914db641fc6dff262fe2504725f2c4c1852b18ffe3588ace693f205000000001976a9141321c4f37c5b2be510c1c7725a83e561ad27876b88ac00000000"
)


func main () {
	// public key script
	b, e := hex.DecodeString(pubScr)
	if e != nil {
		panic(e.Error())
	}
	key, e := btc.NewPublicKey(b[:])
	if e != nil {
		panic(e.Error())
	}
	println("x", key.X.String())
	println("y", key.Y.String())

	// signature script
	b, e = hex.DecodeString(sigScr)
	if e != nil {
		panic(e.Error())
	}
	sig, e := btc.NewSignature(b[:])
	if e != nil {
		panic(e.Error())
	}

	println("r", sig.R.String())
	println("s", sig.S.String())

	// hash of the message
	b, e = hex.DecodeString(msgRaw + "01000000")
	if e != nil {
		panic(e.Error())
	}
	h := btc.NewSha2Hash(b[:])
	println(h.String())
	
	ok := key.Verify(h, sig)
	println(ok)
}
