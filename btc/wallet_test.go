package btc

import (
	"bytes"
	"testing"
	"crypto/rand"
	"encoding/hex"
)


func TestGetPublic(t *testing.T) {
	prv, _ := hex.DecodeString("bb87a5e3e786ecd05f4901ef7ef32726570bfd176ada37a31ef2861db2834d7e")
	pub, _ := hex.DecodeString("02a60d70cfba37177d8239d018185d864b2bdd0caf5e175fd4454cc006fd2d75ac")
	pk, _ := PublicFromPrivate(prv, true)
	if !bytes.Equal(pub, pk) {
		t.Error("PublicFromPrivate failed")
	}
}


func _TestDeterministicWalletType2(t *testing.T) {
	secret := make([]byte, 32)
	rand.Read(secret)

	private_key := make([]byte, 32)
	rand.Read(private_key)

	public_key, _ := PublicFromPrivate(private_key, true)

	//println("p", hex.EncodeToString(private_key.Bytes()))
	//println("q", hex.EncodeToString(secret.Bytes()))

	//println("x", hex.EncodeToString(x.Bytes()))
	//println("y", hex.EncodeToString(y.Bytes()))
	//println(hex.EncodeToString(xy2pk(x, y)))

	var i int
	for i=0; i<50; i++ {
		private_key = DeriveNextPrivate(private_key, secret)
		if private_key==nil {
			t.Fatal("DeriveNextPrivate fail")
		}

		public_key = DeriveNextPublic(public_key, secret)
		if public_key==nil {
			t.Fatal("DeriveNextPublic fail")
		}

		// verify the public key matching the private key
		pub2, _ := PublicFromPrivate(private_key, true)
		if !bytes.Equal(public_key, pub2) {
			t.Error(i, "public key mismatch", hex.EncodeToString(pub2))
		}

		// make sure that you can sign and verify with it
		if e := VerifyKeyPair(private_key, public_key); e!=nil {
			t.Error(i, "verify key failed", e.Error())
		}
	}
}
