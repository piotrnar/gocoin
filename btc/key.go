package btc

import (
	"fmt"
	"errors"
	"bytes"
	"math/big"
	"crypto/ecdsa"
)

type PublicKey struct {
	ecdsa.PublicKey
}


func NewPublicKey(buf []byte) (res *PublicKey, e error) {
	if len(buf)>=65 && buf[0]==4 {
		res = new(PublicKey)
		res.Curve = S256()
		res.X = new(big.Int).SetBytes(buf[1:33])
		res.Y = new(big.Int).SetBytes(buf[33:65])
		return
	}
	e = errors.New("NewPublicKey: Unknown format")
	return
}



func (pk *PublicKey) Verify(h *Uint256, s *Signature) bool {
	return ecdsa.Verify(&pk.PublicKey, h.Hash[:], s.R, s.S)
}


type Signature struct {
	R, S *big.Int
	HashType byte
}


func NewSignature(buf []byte) (sig *Signature, e error) {
	var c byte
	if len(buf)<9 {
		e = errors.New("NewSignature: Key too short " + fmt.Sprint(len(buf)))
		return
	}
	
	rd := bytes.NewReader(buf[:])
	
	// 0x30
	c, e = rd.ReadByte()
	if e!=nil || c!=0x30 {
		e = errors.New("NewSignature: Error parsing Signature at step 1")
		return
	}

	// 0x45
	c, e = rd.ReadByte()
	if e!=nil || int(c)+1 > rd.Len() {
		e = errors.New("NewSignature: Error parsing Signature at step 2")
		return
	}
	
	// 0x02
	c, e = rd.ReadByte()
	if e!=nil || c!=0x02 {
		e = errors.New("NewSignature: Error parsing Signature at step 3")
		return
	}
	
	// len R
	c, e = rd.ReadByte()
	if e!=nil {
		e = errors.New("NewSignature: Error parsing Signature at step 4")
		return
	}
	Rdat := make([]byte, int(c))
	var n int
	n, e = rd.Read(Rdat[:])
	if n!=int(c) || e != nil {
		e = errors.New("NewSignature: Error parsing Signature at step 5")
		return
	}
	
	// 0x02
	c, e = rd.ReadByte()
	if e!=nil || c!=0x02 {
		e = errors.New("NewSignature: Error parsing Signature at step 5a")
		return
	}
	
	// len S
	c, e = rd.ReadByte()
	if e!=nil {
		e = errors.New("NewSignature: Error parsing Signature at step 6")
		return
	}
	Sdat := make([]byte, int(c))
	n, e = rd.Read(Sdat[:])
	if n!=int(c) || e != nil {
		e = errors.New("NewSignature: Error parsing Signature at step 7")
		return
	}

	c, e = rd.ReadByte()
	if e!=nil {
		e = errors.New("NewSignature: Error parsing Signature at step 8")
		return
	}

	sig = new(Signature)
	sig.R = new(big.Int).SetBytes(Rdat[:])
	sig.S = new(big.Int).SetBytes(Sdat[:])
	sig.HashType = c
	
	return
}

