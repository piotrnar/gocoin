package btc

import (
	"bytes"
	"crypto/sha256"
	"github.com/piotrnar/gocoin/lib/others/ripemd160"
)


func ShaHash(b []byte, out []byte) {
	s := sha256.New()
	s.Write(b[:])
	tmp := s.Sum(nil)
	s.Reset()
	s.Write(tmp)
	copy(out[:], s.Sum(nil))
}


// Sha2Sum returns hash: SHA256( SHA256( data ) ).
// Where possible, using ShaHash() should be a bit faster.
func Sha2Sum(b []byte) (out [32]byte) {
	ShaHash(b, out[:])
	return
}


func RimpHash(in []byte, out []byte) {
	sha := sha256.New()
	sha.Write(in)
	rim := ripemd160.New()
	rim.Write(sha.Sum(nil)[:])
	copy(out, rim.Sum(nil))
}


// Rimp160AfterSha256 returns hash: RIMP160( SHA256( data ) ).
// Where possible, using RimpHash() should be a bit faster.
func Rimp160AfterSha256(b []byte) (out [20]byte) {
	RimpHash(b, out[:])
	return
}


// HashFromMessage is used to sign and verify messages using the Bitcoin standard.
// The second parameter must point to a 32-bytes buffer, where the hash will be stored.
func HashFromMessage(msg []byte, out []byte) {
	b := new(bytes.Buffer)
	WriteVlen(b, uint64(len(MessageMagic)))
	b.Write([]byte(MessageMagic))
	WriteVlen(b, uint64(len(msg)))
	b.Write(msg)
	ShaHash(b.Bytes(), out)
}
