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


func BenchmarkDeriveNextPrivate(b *testing.B) {
	var sec [32]byte
	prv := make([]byte, 32)
	ShaHash(sec[:], prv)
	ShaHash(prv, sec[:])
	b.ResetTimer()
	for i:=0; i<b.N; i++ {
		prv = DeriveNextPrivate(prv, sec[:])
	}
}


func BenchmarkDeriveNextPublic(b *testing.B) {
	var prv, sec [32]byte
	ShaHash(prv[:], prv[:])
	ShaHash(prv[:], sec[:])
	pub := PublicFromPrivate(prv[:], true)
	b.ResetTimer()
	for i:=0; i<b.N; i++ {
		pub = DeriveNextPublic(pub, sec[:])
	}
}


func TestDecodePrivateKey(t *testing.T) {
	// mainnet compressed
	pk, er := DecodePrivateAddr("L2zsCZKchUMJ9BS7MVyo8gLGV26rYtgFZskSitwkptk4F1g3KtjN")
	if er != nil {
		t.Error(er.Error())
	}
	if pk.Version != 128 {
		t.Error("Bad version")
	}
	if !pk.IsCompressed() {
		t.Error("Should be compressed")
	}
	if pk.BtcAddr.String()!="179nPBZhnSRM9HB7RM9bJztRAb8ciPitVr" {
		t.Error("Bad address")
	}
	if pk.String() != "L2zsCZKchUMJ9BS7MVyo8gLGV26rYtgFZskSitwkptk4F1g3KtjN" {
		t.Error("Unexpected endode result")
	}

	// testnet uncompressed
	pk, er = DecodePrivateAddr("92fqqcuu2iSqjfAFifVJ7yxDAkUgFEMgu19YgzLxUqXmbJQRrWp")
	if er != nil {
		t.Error(er.Error())
	}
	if pk.Version != 128+0x6f {
		t.Error("Bad version")
	}
	if pk.IsCompressed() {
		t.Error("Should not be compressed")
	}
	if pk.BtcAddr.String()!="mj12iv73R4V5bbyhZg6TTQHfd7bL7rzu2v" {
		t.Error("Bad address")
	}
	if pk.String() != "92fqqcuu2iSqjfAFifVJ7yxDAkUgFEMgu19YgzLxUqXmbJQRrWp" {
		t.Error("Unexpected endode result")
	}

	// litecoin compressed
	pk, er = DecodePrivateAddr("TAtSTnmpQFUKRH56MN7mn6iU8tJpcok7uCP2Hcab599H2pyDZKfY")
	if er != nil {
		t.Error(er.Error())
	}
	if pk.Version != 128+48 {
		t.Error("Bad version")
	}
	if !pk.IsCompressed() {
		t.Error("Should be compressed")
	}
	if pk.BtcAddr.String()!="LMJoWKLk69uXn9joK5LmciyPwiVxAat7Ua" {
		t.Error("Bad address")
	}
	if pk.String() != "TAtSTnmpQFUKRH56MN7mn6iU8tJpcok7uCP2Hcab599H2pyDZKfY" {
		t.Error("Unexpected endode result")
	}

}
