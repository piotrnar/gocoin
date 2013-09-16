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
	if !Verify(key_compr, sig, msg) {
		t.Error("Verify compr failed")
	}

	if Verify(key, sig, msg[1:]) {
		t.Error("Verify not failed")
	}

	if Verify(key_compr, sig, msg[1:]) {
		t.Error("Verify compr not failed")
	}

	k, _ := hex.DecodeString("041f2a00036b3cbd1abe71dca54d406a1e9dd5d376bf125bb109726ff8f2662edcd848bd2c44a86a7772442095c7003248cc619bfec3ddb65130b0937f8311c787")
	k_c, _ := hex.DecodeString("031f2a00036b3cbd1abe71dca54d406a1e9dd5d376bf125bb109726ff8f2662edc")
	s, _ := hex.DecodeString("3045022100ec6eb6b2aa0580c8e75e8e316a78942c70f46dd175b23b704c0330ab34a86a34022067a73509df89072095a16dbf350cc5f1ca5906404a9275ebed8a4ba219627d6701")
	m, _ := hex.DecodeString("7c8e7c2cb887682ed04dc82c9121e16f6d669ea3d57a2756785c5863d05d2e6a")
	if !Verify(k, s, m) {
		t.Error("Verify2 failed")
	}
	if !Verify(k_c, s, m) {
		t.Error("Verify2 compr failed")
	}
	if Verify(k, s, m[1:]) {
		t.Error("Verify2 not failed")
	}
	if Verify(k_c, s, m[1:]) {
		t.Error("Verify2 compr not failed")
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
