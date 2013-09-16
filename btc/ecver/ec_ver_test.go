package ecver

import (
	"testing"
	"encoding/hex"
	"github.com/piotrnar/gocoin/btc"
)


var (
	key, key_compr, sig, msg []byte
)


func init() {
	key, _ = hex.DecodeString("040eaebcd1df2df853d66ce0e1b0fda07f67d1cabefde98514aad795b86a6ea66dbeb26b67d7a00e2447baeccc8a4cef7cd3cad67376ac1c5785aeebb4f6441c16")
	key_compr, _ = hex.DecodeString("020eaebcd1df2df853d66ce0e1b0fda07f67d1cabefde98514aad795b86a6ea66d")
	sig, _ = hex.DecodeString("3045022100fe00e013c244062847045ae7eb73b03fca583e9aa5dbd030a8fd1c6dfcf11b1002207d0d04fed8fa1e93007468d5a9e134b0a7023b6d31db4e50942d43a250f4d07c01")
	msg, _ = hex.DecodeString("3382219555ddbb5b00e0090f469e590ba1eae03c7f28ab937de330aa60294ed6")
}

func TestTheVerify(t *testing.T) {
	if !Verify(key, sig, msg) {
		t.Error("Verify failed")
	}

	msg[0]++
	if Verify(key, sig, msg) {
		t.Error("Verify not failed")
	}
}

func BenchmarkVerifyUncompressed(b *testing.B) {
	for i := 0; i < b.N; i++ {
		Verify(key, sig, msg)
	}
}

func BenchmarkVerifyCompressed(b *testing.B) {
	for i := 0; i < b.N; i++ {
		Verify(key_compr, sig, msg)
	}
}

func BenchmarkVerifyRawIntegers(b *testing.B) {
	pk, _ := btc.NewPublicKey(key)
	s, _ := btc.NewSignature(sig)

	var sign secp256k1_ecdsa_sig_t
	var pkey secp256k1_ge_t
	var mesg secp256k1_num_t

	sign.r.Set(s.R)
	sign.s.Set(s.S)

	pkey.x.Set(pk.X)
	pkey.y.Set(pk.Y)

	mesg.SetBytes(msg)

	for i := 0; i < b.N; i++ {
		sign.verify(&pkey, &mesg)
	}
}
