package btc

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"math/big"
)

const Uint256IdxLen = 8 // The bigger it is, the more memory is needed, but lower chance of a collision

type Uint256 struct {
	Hash [32]byte
}

func NewUint256(h []byte) (res *Uint256) {
	res = new(Uint256)
	copy(res.Hash[:], h)
	return
}

// Get from MSB hexstring
func NewUint256FromString(s string) (res *Uint256) {
	d, e := hex.DecodeString(s)
	if e != nil {
		//println("NewUint256FromString", s, e.Error())
		return
	}
	if len(d) != 32 {
		//println("NewUint256FromString", s, "not 32 bytes long")
		return
	}
	res = new(Uint256)
	for i := 0; i < 32; i++ {
		res.Hash[31-i] = d[i]
	}
	return
}

func NewSha2Hash(data []byte) (res *Uint256) {
	res = new(Uint256)
	ShaHash(data, res.Hash[:])
	return
}

func (u *Uint256) Bytes() []byte {
	return u.Hash[:]
}

func (u *Uint256) String() (s string) {
	for i := 0; i < 32; i++ {
		s += fmt.Sprintf("%02x", u.Hash[31-i])
	}
	return
}

func (u *Uint256) Equal(o *Uint256) bool {
	return bytes.Equal(u.Hash[:], o.Hash[:])
}

func (u *Uint256) Calc(data []byte) {
	ShaHash(data, u.Hash[:])
}

func BIdx(hash []byte) (o [Uint256IdxLen]byte) {
	copy(o[:], hash[:Uint256IdxLen])
	return
}

func (u *Uint256) BIdx() (o [Uint256IdxLen]byte) {
	o = BIdx(u.Hash[:])
	return
}

func BIdxString(o [Uint256IdxLen]byte) (s string) {
	s = fmt.Sprintf("%016x", binary.LittleEndian.Uint64(o[:]))
	return
}

func (u *Uint256) BigInt() *big.Int {
	var buf [32]byte
	for i := range buf {
		buf[i] = u.Hash[31-i]
	}
	return new(big.Int).SetBytes(buf[:])
}
