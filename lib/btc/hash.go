package btc

import (
	"bytes"
	"crypto/sha256"
	"encoding"
	"hash"

	"github.com/piotrnar/gocoin/lib/others/ripemd160"
)

const (
	HASHER_TAPSIGHASH = 0
	HASHER_TAPLEAF    = 1
	HASHER_TAPBRANCH  = 2
	HASHER_TAPTWEAK   = 3
)

var (
	hash_tags = []string{"TapSighash", "TapLeaf", "TapBranch", "TapTweak"}
	hashers   [HASHER_TAPTWEAK + 1][]byte
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

type HMAC_SHA256 struct {
	inner, outer hash.Hash
}

type RFC6979_HMAC_SHA256 struct {
	v, k  [32]byte
	retry bool
}

func HMAC_Init(key []byte) (res *HMAC_SHA256) {
	var rkey [64]byte
	res = new(HMAC_SHA256)
	if len(key) < 64 {
		copy(rkey[:], key)
	} else {
		sha := sha256.New()
		sha.Write(key)
		copy(rkey[:], sha.Sum(nil))
	}
	res.outer = sha256.New()
	for i := range rkey {
		rkey[i] ^= 0x5c
	}
	res.outer.Write(rkey[:])

	res.inner = sha256.New()
	for i := range rkey {
		rkey[i] ^= 0x5c ^ 0x36
	}
	res.inner.Write(rkey[:])
	ClearBuffer(rkey[:])
	return
}

func (res *HMAC_SHA256) Write(p []byte) {
	res.inner.Write(p)
}

func (res *HMAC_SHA256) Finalize(out []byte) {
	temp := res.inner.Sum(nil)
	res.outer.Write(temp)
	ClearBuffer(temp)
	copy(out, res.outer.Sum(nil))
}

func RFC6979_HMAC_Init(key []byte) (rng *RFC6979_HMAC_SHA256) {
	rng = new(RFC6979_HMAC_SHA256)
	FillBuffer(rng.v[:], 1) /* RFC6979 3.2.b. */

	/* RFC6979 3.2.d. */
	hmac := HMAC_Init(rng.k[:])
	hmac.Write(rng.v[:])
	hmac.Write([]byte{0})
	hmac.Write(key)
	hmac.Finalize(rng.k[:])
	hmac = HMAC_Init(rng.k[:])
	hmac.Write(rng.v[:])
	hmac.Finalize(rng.v[:])

	/* RFC6979 3.2.f. */
	hmac = HMAC_Init(rng.k[:])
	hmac.Write(rng.v[:])
	hmac.Write([]byte{1})
	hmac.Write(key)
	hmac.Finalize(rng.k[:])
	hmac = HMAC_Init(rng.k[:])
	hmac.Write(rng.v[:])
	hmac.Finalize(rng.v[:])
	return
}

func (rng *RFC6979_HMAC_SHA256) Generate(out []byte) {
	var hmac *HMAC_SHA256
	var outlen int

	if rng.retry {
		hmac = HMAC_Init(rng.k[:])
		hmac.Write(rng.v[:])
		hmac.Write([]byte{0})
		hmac.Finalize(rng.k[:])
		hmac = HMAC_Init(rng.k[:])
		hmac.Write(rng.v[:])
		hmac.Finalize(rng.v[:])
	}

	for outlen < len(out) {
		hmac = HMAC_Init(rng.k[:])
		hmac.Write(rng.v[:])
		hmac.Finalize(rng.v[:])
		copy(out[outlen:], rng.v[:])
		outlen += 32
	}

	rng.retry = true
}

func (rng *RFC6979_HMAC_SHA256) Finalize() {
	ClearBuffer(rng.v[:])
	ClearBuffer(rng.k[:])
}

func ClearBuffer(b []byte) {
	for i := range b {
		b[i] = 0
	}
}

func FillBuffer(b []byte, v byte) {
	for i := range b {
		b[i] = v
	}
}
