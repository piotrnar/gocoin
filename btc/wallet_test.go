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
	pk := PublicFromPrivate(prv, true)
	if !bytes.Equal(pub, pk) {
		t.Error("PublicFromPrivate failed")
	}
}


func TestDeterministicPublic(t *testing.T) {
	secret, _ := hex.DecodeString("4438addb9b147349432466d89d81f4dae1fc1fd9bcb764d2854f303931796c2d")
	pubkey, _ := hex.DecodeString("03578936ea365dd8921fe0e05eb4d2af9a0d333312ec01ae950f9450af09cef4d4")
	exp, _ := hex.DecodeString("03ba29c4d2168af9d8e4492d158ff76455999f94333f47bc15275a2586db6d491d")
	pubkey = DeriveNextPublic(pubkey, secret)
	if !bytes.Equal(pubkey, exp) {
		t.Error("TestDeterministicPublic failed")
	}
}


func TestDeterministicWalletType2(t *testing.T) {
	secret := make([]byte, 32)
	rand.Read(secret)

	private_key := make([]byte, 32)
	rand.Read(private_key)

	public_key := PublicFromPrivate(private_key, true)
	for i:=0; i<50; i++ {
		private_key = DeriveNextPrivate(private_key, secret)
		if private_key==nil {
			t.Fatal("DeriveNextPrivate fail")
		}

		public_key = DeriveNextPublic(public_key, secret)
		if public_key==nil {
			t.Fatal("DeriveNextPublic fail")
		}

		// verify the public key matching the private key
		pub2 := PublicFromPrivate(private_key, true)
		if !bytes.Equal(public_key, pub2) {
			t.Error(i, "public key mismatch", hex.EncodeToString(pub2), hex.EncodeToString(public_key))
		}

		// make sure that you can sign and verify with it
		if e := VerifyKeyPair(private_key, public_key); e!=nil {
			t.Error(i, "verify key failed", e.Error())
		}
	}
}

func BenchmarkPrivToPubCompr(b *testing.B) {
	var prv [32]byte
	for i:=0; i<b.N; i++ {
		ShaHash(prv[:], prv[:])
		PublicFromPrivate(prv[:], true)
	}
}


func BenchmarkPrivToPubUncom(b *testing.B) {
	var prv [32]byte
	for i:=0; i<b.N; i++ {
		ShaHash(prv[:], prv[:])
		PublicFromPrivate(prv[:], false)
	}
}
