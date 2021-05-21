package secp256k1

import (
	"bytes"
	"crypto/sha256"
	"encoding"
	"hash"
)

func SchnorrsigChallenge(e *Number, r32, msg32, pubkey32 []byte) {
	s := ShaMidstate()

	s.Write(r32)
	s.Write(pubkey32)
	s.Write(msg32)
	e.SetBytes(s.Sum(nil))
}

func SchnorrVerify(pkey, sig, msg []byte) (ret bool) {
	var rx Field
	var pk, r XY
	var rj, pkj XYZ
	var _s, _e Number

	rx.SetB32(sig[:32])
	pk.ParseXOnlyPubkey(pkey)

	SchnorrsigChallenge(&_e, sig[:32], msg, pkey)
	_e.sub(&TheCurve.Order, &_e)

	_s.SetBytes(sig[32:])
	pkj.SetXY(&pk)
	pkj.ECmult(&rj, &_e, &_s)

	r.SetXYZ(&rj)
	if r.Infinity {
		return false
	}

	r.Y.Normalize()
	if r.Y.IsOdd() {
		return false
	}

	r.X.Normalize()
	return rx.Equals(&r.X)
}

func CheckPayToContract(m_keydata, base, hash []byte, parity bool) bool {
	var base_point XY
	base_point.ParseXOnlyPubkey(base)
	return base_point.XOnlyPubkeyTweakAddCheck(m_keydata, parity, hash)
}

func (pk *XY) XOnlyPubkeyTweakAddCheck(tweaked_pubkey32 []byte, tweaked_pk_parity bool, hash []byte) bool {
	var pk_expected32 [32]byte
	var tweak Number

	tweak.SetBytes(hash)
	if !pk.ECPublicTweakAdd(&tweak) {
		return false
	}
	pk.X.Normalize()
	pk.Y.Normalize()
	pk.X.GetB32(pk_expected32[:])

	if bytes.Equal(pk_expected32[:], tweaked_pubkey32) {
		if pk.Y.IsOdd() == tweaked_pk_parity {
			return true
		}
	}

	return false
}

func (key *XY) ECPublicTweakAdd(tweak *Number) bool {
	var pt, pt2 XYZ
	var one Number
	pt.SetXY(key)
	one.SetInt64(1)
	pt.ECmult(&pt2, &one, tweak)
	if pt2.IsInfinity() {
		return false
	}
	key.SetXYZ(&pt2)
	return true
}

var _sha_midstate []byte

func ShaMidstate() hash.Hash {
	s := sha256.New()
	unmarshaler, ok := s.(encoding.BinaryUnmarshaler)
	if !ok {
		panic("second does not implement encoding.BinaryUnmarshaler")
	}
	if err := unmarshaler.UnmarshalBinary(_sha_midstate); err != nil {
		panic("unable to unmarshal hash: " + err.Error())
	}
	return s
}

func init() {
	s := sha256.New()
	s.Write([]byte("BIP0340/challenge"))
	c := s.Sum(nil)
	s.Reset()
	s.Write(c)
	s.Write(c)

	var err error
	marshaler, ok := s.(encoding.BinaryMarshaler)
	if !ok {
		panic("first does not implement encoding.BinaryMarshaler")
	}
	_sha_midstate, err = marshaler.MarshalBinary()
	if err != nil {
		panic("unable to marshal hash: " + err.Error())
	}
}
