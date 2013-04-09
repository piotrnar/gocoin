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

func decompressPoint(off bool, x *big.Int)  (y *big.Int) {
	x2 := new(big.Int).Mul(x, x)
	x3 := new(big.Int).Mul(x2, x)

	y2 := new(big.Int).Add(x3, secp256k1.B)

	y = new(big.Int).Exp(y2, Qplus1div4, secp256k1.P)

	bts := y.Bytes()
	odd := (bts[len(bts)-1]&1)!=0

	if odd != off {
		y = new(big.Int).Sub(secp256k1.P, y)
	}

	return
}


/*
Public keys (in scripts) are given as 04 <x> <y> where x and y are 32 byte big-endian 
integers representing the coordinates of a point on the curve 

or in compressed form given as <sign> <x> where <sign> is 0x02 if y is even and 0x03 if y is odd.
*/

func NewPublicKey(buf []byte) (res *PublicKey, e error) {
	//fmt.Println("Het Pub Key:", hex.EncodeToString(buf[:]))
	if len(buf)==65 && buf[0]==4 {
		res = new(PublicKey)
		res.Curve = S256()
		res.X = new(big.Int).SetBytes(buf[1:33])
		res.Y = new(big.Int).SetBytes(buf[33:65])
		return
	}
	if len(buf)==33 && (buf[0]==2 || buf[0]==3) {
		//println("Warning: comperssed public key")
		res = new(PublicKey)
		res.Curve = S256()
		res.X = new(big.Int).SetBytes(buf[1:33])
		res.Y = decompressPoint(buf[0]==3, res.X);
		/*if buf[0]==2 {
			res.Y = decompressPoint(false, res.X);
		} else {
			res.Y = decompressPoint(true, res.X);
		}*/
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

