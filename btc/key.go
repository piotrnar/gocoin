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
	if don(DBG_VERIFY) {
		fmt.Println("Verify signature, HashType", s.HashType)
		fmt.Println("R:", s.R.String())
		fmt.Println("S:", s.S.String())
		fmt.Println("Hash:", hex.EncodeToString(h))
		fmt.Println("Key:", hex.EncodeToString(pk.Bytes(false)))
	}
	ok = ecdsa.Verify(&pk.PublicKey, h, s.R, s.S)
	if don(DBG_VERIFY) {
		fmt.Println("Verify signature =>", ok)
	}
	return
}


type Signature struct {
	R, S *big.Int
	HashType byte
}


func NewSignature(buf []byte) (sig *Signature, e error) {
	var c byte
	if len(buf)<9 || len(buf)>73 {
		e = errors.New("NewSignature: Unexpected signature length ")
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

	/*
	   It seems the implementation in the original bitcoin client has been fucked up.
	   If we'd follow the spec here, we should reject this TX from block #135405:
	       67e758b27df26ad609f943b30e5bbb270d835b737c8b3df1a7944ba08df8b9a2
	   So we must ignore the prior lengths and always take the last byte as the HashType.
	*/
	_, e = rd.ReadByte()
	if e == nil {
		// There is at least one more byte after S, so take the HashType from the last byte
		sig.HashType = buf[len(buf)-1] & 0x7F // &0x7F clears SIGHASH_ANYONECANPAY bit
	} else {
		// A missing HashType byte is not an error - such signatures are used for alerts.
		e = nil
		// sig.HashType field has been set to zero in new(Signature).
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
