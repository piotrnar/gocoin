package secp256k1

import (
	"testing"
	"crypto/rand"
)


func TestFeInv(t *testing.T) {
	var in, out, exp Fe_t
	in.SetHex("813925AF112AAB8243F8CCBADE4CC7F63DF387263028DE6E679232A73A7F3C31")
	exp.SetHex("7F586430EA30F914965770F6098E492699C62EE1DF6CAFFA77681C179FDF3117")
	in.inv(&out)
	if !out.Equals(&exp) {
		t.Error("fe.inv() failed")
	}
}

func BenchmarkFieldSqrt(b *testing.B) {
	var dat [32]byte
	var f, tmp Fe_t
	rand.Read(dat[:])
	f.SetB32(dat[:])
	for i := 0; i < b.N; i++ {
		f.sqrt(&tmp)
	}
}


func BenchmarkFieldInv(b *testing.B) {
	var dat [32]byte
	var f, tmp Fe_t
	rand.Read(dat[:])
	f.SetB32(dat[:])
	for i := 0; i < b.N; i++ {
		f.inv(&tmp)
	}
}

