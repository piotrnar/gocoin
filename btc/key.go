package btc

import (
	"bytes"
	"errors"
	"encoding/hex"
	"github.com/piotrnar/gocoin/secp256k1"
)

type PublicKey struct {
	secp256k1.XY
}


/*
Public keys (in scripts) are given as 04 <x> <y> where x and y are 32 byte big-endian
integers representing the coordinates of a point on the curve

or in compressed form given as <sign> <x> where <sign> is 0x02 if y is even and 0x03 if y is odd.
*/

func NewPublicKey(buf []byte) (res *PublicKey, e error) {
	res = new(PublicKey)
	if !res.XY.ParsePubkey(buf) {
		e = errors.New("NewPublicKey: Unknown format: "+hex.EncodeToString(buf[:]))
		res = nil
	}
	return
}


type Signature struct {
	secp256k1.Signature
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
	sig.R.SetBytes(Rdat[:])
	sig.S.SetBytes(Sdat[:])

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
	key = new(PublicKey)
	if !secp256k1.RecoverPublicKey(sig.R.Bytes(), sig.S.Bytes(), msg, recid, &key.XY) {
		key = nil
	}
	return
}


// Returns serialized canoncal signature
func (sig *Signature) Bytes() []byte {
	return append(sig.Signature.Bytes(), sig.HashType)
}
