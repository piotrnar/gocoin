package secp256k1

import (
	"crypto/rand"
	"testing"
)

func TestFeInv(t *testing.T) {
	var in, out, exp Field
	in.SetHex("813925AF112AAB8243F8CCBADE4CC7F63DF387263028DE6E679232A73A7F3C31")
	exp.SetHex("7F586430EA30F914965770F6098E492699C62EE1DF6CAFFA77681C179FDF3117")
	in.Inv(&out)
	if !out.Equals(&exp) {
		t.Error("fe.Inv() failed")
	}
}

func BenchmarkFieldSqrt(b *testing.B) {
	var dat [32]byte
	var f, tmp Field
	rand.Read(dat[:])
	f.SetB32(dat[:])
	for i := 0; i < b.N; i++ {
		f.Sqrt(&tmp)
	}
}

func BenchmarkFieldInv(b *testing.B) {
	var dat [32]byte
	var f, tmp Field
	rand.Read(dat[:])
	f.SetB32(dat[:])
	for i := 0; i < b.N; i++ {
		f.Inv(&tmp)
	}
}

func BenchmarkFieldSqr(b *testing.B) {
	var dat [32]byte
	var f, tmp Field
	rand.Read(dat[:])
	f.SetB32(dat[:])
	for i := 0; i < b.N; i++ {
		f.Sqr(&tmp)
	}
}

func BenchmarkFieldMul(b *testing.B) {
	var dat [64]byte
	var f, g, tmp Field
	rand.Read(dat[:])
	f.SetB32(dat[:32])
	g.SetB32(dat[32:])
	for i := 0; i < b.N; i++ {
		f.Mul(&tmp, &g)
	}
}
