package btc

import (
	"fmt"
	"bytes"
    "math/big"
)

const Uint256IdxLen = 6  // The bigger it is, the more memory is needed, but lower chance of a collision

type Uint256 struct {
	Hash [32]byte
}

func NewUint256(h []byte) (res *Uint256) {
	res = new(Uint256)
	copy(res.Hash[:], h[:])
	return
}

// Get from MSB hexstring
func NewUint256FromString(s string) (res *Uint256) {
	var v int
	res = new(Uint256)
	for i := 0; i<32; i++ {
		fmt.Sscanf(s[2*i:2*i+2], "%x", &v)
		res.Hash[31-i] = byte(v)
	}
	return
}

// Get from LSB hexstring
func NewUint256FromLSBString(s string) (res *Uint256) {
	var v int
	res = new(Uint256)
	for i := 0; i<32; i++ {
		fmt.Sscanf(s[2*i:2*i+2], "%x", &v)
		res.Hash[i] = byte(v)
	}
	return
}

func NewSha2Hash(data []byte) (res *Uint256) {
	res = new(Uint256)
	res.Hash = Sha2Sum(data[:])
	return
}

func (u *Uint256) String() (s string) {
	for i := 0; i<32; i++ {
		s+= fmt.Sprintf("%02x", u.Hash[31-i])
	}
	return
}

func (u *Uint256) Equal(o *Uint256) bool {
	return bytes.Equal(u.Hash[:], o.Hash[:])
}

func (u *Uint256) BIdx() [Uint256IdxLen]byte {
	return NewBlockIndex(u.Hash[:])
}

func (u *Uint256) BigInt() *big.Int {
	var buf [32]byte
	for i := range buf {
		buf[i] = u.Hash[31-i]
	}
	return new(big.Int).SetBytes(buf[:])
}


