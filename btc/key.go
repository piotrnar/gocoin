package btc

import (
	"fmt"
	"errors"
	"bytes"
	"math/big"
	"crypto/ecdsa"
	"encoding/hex"
)

type PublicKey struct {
	ecdsa.PublicKey
}


/*
 Thanks @Zeilap
 https://bitcointalk.org/index.php?topic=171314.msg1781562#msg1781562
*/
func decompressPoint(off bool, x *big.Int)  (y *big.Int) {
	x3 := new(big.Int).Mul(x, x) //x^2
	x3.Mul(x3, x)                //x^3

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
	switch len(buf) {
		case 65:
			/*if buf[0]==4*/ {
				res = new(PublicKey)
				res.Curve = S256()
				res.X = new(big.Int).SetBytes(buf[1:33])
				res.Y = new(big.Int).SetBytes(buf[33:65])
				return
			}
		case 33:
			/*if buf[0]==2 || buf[0]==3*/ {
				res = new(PublicKey)
				res.Curve = S256()
				res.X = new(big.Int).SetBytes(buf[1:33])
				res.Y = decompressPoint(buf[0]==3, res.X);
				return
			}
	}
	e = errors.New("NewPublicKey: Unknown format: "+hex.EncodeToString(buf[:]))
	return
}


// Returns serialized key in uncompressed format "<04> <X> <Y>"
// ... or in compressed format: "<02> <X>", eventually "<03> <X>"
func (pub *PublicKey) Bytes(compressed bool) (raw []byte) {
	if compressed {
		raw = make([]byte, 33)
		raw[0] = byte(2+pub.Y.Bit(0))
		x := pub.X.Bytes()
		copy(raw[1+32-len(x):], x)
	} else {
		raw = make([]byte, 65)
		raw[0] = 4
		x := pub.X.Bytes()
		y := pub.Y.Bytes()
		copy(raw[1+32-len(x):], x)
		copy(raw[1+64-len(y):], y)
	}
	return
}


func (pk *PublicKey) Verify(h []byte, s *Signature) (ok bool) {
	ok = ecdsa.Verify(&pk.PublicKey, h[:], s.R, s.S)
	return
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
	if e!=nil || int(c) > rd.Len() {
		println(e, c, rd.Len())
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

	sig = new(Signature)
	sig.R = new(big.Int).SetBytes(Rdat[:])
	sig.S = new(big.Int).SetBytes(Sdat[:])

	c, e = rd.ReadByte()
	if e==nil {
		sig.HashType = c
	} else {
		e = nil // missing hash type byte is not an error (i.e. for alert signature)
	}

	return
}


/*
Thanks to jackjack for providing me with this nice solution:
https://bitcointalk.org/index.php?topic=162805.msg2112936#msg2112936
*/
func (sig *Signature) RecoverPublicKey(msg []byte, recid int) (key *PublicKey) {
	x := new(big.Int).Set(secp256k1.N)
	x.Mul(x, big.NewInt(int64(recid/2)))
	x.Add(x, sig.R)

	y := decompressPoint((recid&1)!=0, x)

	e := new(big.Int).SetBytes(msg)
	new(big.Int).DivMod(e.Neg(e), secp256k1.N, e)

	_x, _y := secp256k1.ScalarMult(x, y, sig.S.Bytes())
	x, y = secp256k1.ScalarMult(secp256k1.Gx, secp256k1.Gy, e.Bytes())
	_x, _y = secp256k1.Add(_x, _y, x, y)
	x, y = secp256k1.ScalarMult(_x, _y, new(big.Int).ModInverse(sig.R, secp256k1.N).Bytes())

	key = new(PublicKey)
	key.Curve = secp256k1
	key.X = x
	key.Y = y

	return
}
