package btc

import (
	"errors"
	"bytes"
	"math/big"
	"crypto/ecdsa"
	"encoding/hex"
	"github.com/piotrnar/gocoin/btc/newec"
)

type PublicKey struct {
	ecdsa.PublicKey
}


/*
Public keys (in scripts) are given as 04 <x> <y> where x and y are 32 byte big-endian
integers representing the coordinates of a point on the curve

or in compressed form given as <sign> <x> where <sign> is 0x02 if y is even and 0x03 if y is odd.
*/

func NewPublicKey(buf []byte) (res *PublicKey, e error) {
	switch len(buf) {
		case 65:
			if buf[0]==4 {
				res = new(PublicKey)
				res.Curve = S256()
				res.X = new(big.Int).SetBytes(buf[1:33])
				res.Y = new(big.Int).SetBytes(buf[33:65])
				return
			}
		case 33:
			if buf[0]==2 || buf[0]==3 {
				var y [32]byte
				newec.DecompressPoint(buf[1:33], buf[0]==0x03, y[:])
				res = new(PublicKey)
				res.Curve = S256()
				res.X = new(big.Int).SetBytes(buf[1:33])
				res.Y = new(big.Int).SetBytes(y[:])
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


type Signature struct {
	R, S *big.Int
	HashType byte
}


func NewSignature(buf []byte) (sig *Signature, e error) {
	var c byte
	if len(buf)<9 {
		e = errors.New("NewSignature: Signature too short")
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
		sig.HashType = buf[len(buf)-1]
	} else {
		// A missing HashType byte is not an error - such signatures are used for alerts.
		e = nil
		// sig.HashType field has been set to zero in new(Signature).
	}

	return
}


// Recoved public key form a signature
func (sig *Signature) RecoverPublicKey(msg []byte, recid int) (key *PublicKey) {
	var x, y [32]byte
	if newec.RecoverPublicKey(sig.R.Bytes(), sig.S.Bytes(), msg, recid, x[:], y[:]) {
		key = new(PublicKey)
		key.X = new(big.Int).SetBytes(x[:])
		key.Y = new(big.Int).SetBytes(y[:])
	}
	return
}


// Returns serialized canoncal signature
func (sig *Signature) Bytes() []byte {
	r := sig.R.Bytes()
	if r[0]>=0x80 {
		r = append([]byte{0}, r...)
	}
	s := sig.S.Bytes()
	if s[0]>=0x80 {
		s = append([]byte{0}, s...)
	}
	res := new(bytes.Buffer)
	res.WriteByte(0x30)
	res.WriteByte(byte(4+len(r)+len(s)))
	res.WriteByte(0x02)
	res.WriteByte(byte(len(r)))
	res.Write(r)
	res.WriteByte(0x02)
	res.WriteByte(byte(len(s)))
	res.Write(s)
	res.WriteByte(sig.HashType)
	return res.Bytes()
}
