package secp256k1

import (
	"bytes"
	"crypto/sha256"
	"encoding"
	"encoding/hex"
	"hash"
)

func SchnorrsigChallenge(e *Number, r32, msg32, pubkey32 []byte) {
	s := ShaMidstateChallenge()

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

func get_n_minus(in []byte) []byte {
	var n Number
	n.SetBytes(in)
	n.sub(&TheCurve.Order, &n)
	return n.get_bin(32)
}

func SchnorrSign(m, sk, a []byte) []byte {
	var xyz XYZ
	var n, x Number
	var P, R XY
	var d, t, k, e, res []byte

	n.SetBytes(sk) // d
	if n.is_zero() || !n.is_below(&TheCurve.Order) {
		println("SchnorrSign: d out of range")
		return nil
	}
	ECmultGen(&xyz, &n)
	P.SetXYZ(&xyz)
	P.Y.Normalize()
	if P.Y.IsOdd() {
		d = get_n_minus(sk)
	} else {
		d = sk
	}

	s := ShaMidstateAux()
	s.Write(a)
	t = s.Sum(nil)
	for i := range t {
		t[i] ^= d[i]
	}

	s = ShaMidstateNonce()
	s.Write(t)
	P.X.Normalize()
	P.X.GetB32(t)
	s.Write(t)
	s.Write(m)
	k0 := s.Sum(nil)

	n.SetBytes(k0)
	n.mod(&TheCurve.Order)
	if n.is_zero() {
		println("SchnorrSign: k' is zero")
		return nil
	}
	ECmultGen(&xyz, &n)
	R.SetXYZ(&xyz)
	R.Y.Normalize()
	if R.Y.IsOdd() {
		k = get_n_minus(k0)
	} else {
		k = k0
	}

	res = make([]byte, 64)
	R.X.Normalize()
	P.X.Normalize()
	R.X.GetB32(res[:32])
	P.X.GetB32(res[32:])
	copy(t, res[32:]) // save public key for the verify function
	s = ShaMidstateChallenge()
	s.Write(res)
	s.Write(m)
	e = s.Sum(nil)

	n.SetBytes(e)
	if !n.is_below(&TheCurve.Order) {
		n.sub(&n, &TheCurve.Order) // we need to use "e mod N"
	}

	// signature: ((e * d + k) mod N)
	x.SetBytes(d)
	n.mul(&n, &x)

	x.SetBytes(k)
	n.add(&n, &x)
	n.mod(&TheCurve.Order)

	copy(res[32:], n.get_bin(32))
	if !SchnorrVerify(t, res, m) {
		println("SchnorrSign: verify error", hex.EncodeToString(res))
		return nil
	}
	return res
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

var _sha_midstate_challenge, _sha_midstate_aux, _sha_midstate_nonce []byte

func ShaMidstateChallenge() hash.Hash {
	s := sha256.New()
	unmarshaler, _ := s.(encoding.BinaryUnmarshaler)
	unmarshaler.UnmarshalBinary(_sha_midstate_challenge)
	return s
}

func ShaMidstateAux() hash.Hash {
	s := sha256.New()
	unmarshaler, _ := s.(encoding.BinaryUnmarshaler)
	unmarshaler.UnmarshalBinary(_sha_midstate_aux)
	return s
}

func ShaMidstateNonce() hash.Hash {
	s := sha256.New()
	unmarshaler, _ := s.(encoding.BinaryUnmarshaler)
	unmarshaler.UnmarshalBinary(_sha_midstate_nonce)
	return s
}

func init() {
	s := sha256.New()
	s.Write([]byte("BIP0340/challenge"))
	c := s.Sum(nil)
	s.Reset()
	s.Write(c)
	s.Write(c)
	marshaler, _ := s.(encoding.BinaryMarshaler)
	_sha_midstate_challenge, _ = marshaler.MarshalBinary()

	s.Reset()
	s.Write([]byte("BIP0340/aux"))
	c = s.Sum(nil)
	s.Reset()
	s.Write(c)
	s.Write(c)
	marshaler, _ = s.(encoding.BinaryMarshaler)
	_sha_midstate_aux, _ = marshaler.MarshalBinary()

	s.Reset()
	s.Write([]byte("BIP0340/nonce"))
	c = s.Sum(nil)
	s.Reset()
	s.Write(c)
	s.Write(c)
	marshaler, _ = s.(encoding.BinaryMarshaler)
	_sha_midstate_nonce, _ = marshaler.MarshalBinary()
}
