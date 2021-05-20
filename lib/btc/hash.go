package btc

import (
	"bytes"
	"crypto/sha256"
	"encoding"
	"github.com/piotrnar/gocoin/lib/others/ripemd160"
	"hash"
)

const (
	HASHER_TAPSIGHASH = 0
	HASHER_TAPLEAF    = 1
	HASHER_TAPBRANCH  = 2
	HASHER_TAPTWEAK   = 3
)

var (
	hash_tags = []string{"TapSighash", "TapLeaf", "TapBranch", "TapTweak"}
	hashers [HASHER_TAPTWEAK + 1][]byte
)

func _TaggedHash(tag string) hash.Hash {
	sha := sha256.New()
	sha.Write([]byte(tag))
	taghash := sha.Sum(nil)
	sha.Reset()
	sha.Write(taghash)
	sha.Write(taghash)
	return sha
}

func Hasher(idx int) hash.Hash {
	s := sha256.New()
	unmarshaler, _ := s.(encoding.BinaryUnmarshaler)
	unmarshaler.UnmarshalBinary(hashers[idx])
	return s
}

func init() {
	for i, t := range hash_tags {
		sha := _TaggedHash(t)
		marshaler, _ := sha.(encoding.BinaryMarshaler)
		hashers[i], _ = marshaler.MarshalBinary()
	}
}

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
