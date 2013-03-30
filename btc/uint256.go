package btc

import (
	"fmt"
	"bytes"
)

type Uint256 struct {
	Hash [32]byte
}

func NewUint256(h []byte) (res *Uint256) {
	res = new(Uint256)
	copy(res.Hash[:], h[:])
	return
}

func NewUint256FromString(s string) (res *Uint256) {
	var v int
	res = new(Uint256)
	for i := 0; i<32; i++ {
		fmt.Sscanf(s[2*i:2*i+2], "%x", &v)
		res.Hash[31-i] = byte(v)
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

func (u *Uint256) BIdx() [blockMapLen]byte {
	return NewBlockIndex(u.Hash[:])
}
