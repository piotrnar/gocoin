package btc

import (
	"fmt"
	"errors"
	"math/big"
	"github.com/piotrnar/gocoin/secp256k1"
)


// Get ECDSA public key in bitcoin protocol format, from the give private key
func PublicFromPrivate(priv_key []byte, compressed bool) (res []byte, e error) {
	x, y := temp_secp256k1.ScalarBaseMult(priv_key)
	xd := x.Bytes()

	if len(xd)>32 {
		e = errors.New(fmt.Sprint("PublicFromPrivate: x is too long", len(xd)))
		return
	}

	if !compressed {
		yd := y.Bytes()
		if len(yd)>32 {
			e = errors.New(fmt.Sprint("PublicFromPrivate: y is too long", len(yd)))
			return
		}

		res = make([]byte, 65)
		res[0] = 4
		copy(res[1+32-len(xd):33], xd)
		copy(res[33+32-len(yd):65], yd)
	} else {
		res = make([]byte, 33)
		res[0] = 2+byte(y.Bit(0)) // 02 for even Y values, 03 for odd..
		copy(res[1+32-len(xd):33], xd)
	}

	return
}


// Verify the secret key's range and if a test message signed with it verifies OK
// Returns nil if everything looks OK
func VerifyKeyPair(priv []byte, publ []byte) error {
	var e error
	var sig Signature

	const TestMessage = "Just some test message..."
	hash := Sha2Sum([]byte(TestMessage))

	D := new(big.Int).SetBytes(priv)

	if D.Cmp(big.NewInt(0)) == 0 {
		return errors.New("pubkey value is zero")
	}

	if D.Cmp(&secp256k1.TheCurve.Order.Int) != -1 {
		return errors.New("pubkey value is too big")
	}


	sig.R, sig.S, e = EcdsaSign(priv, hash[:])
	if e != nil {
		return errors.New("EcdsaSign failed: " + e.Error())
	}

	ok := EcdsaVerify(publ, sig.Bytes(), hash[:])
	if !ok {
		return errors.New("EcdsaVerify failed")
	}
	return nil
}

// B_private_key = ( A_private_key + secret ) % N
// Used for implementing Type-2 determinitic keys
func DeriveNextPrivate(prv, secret *big.Int) *big.Int {
	return new(big.Int).Mod(new(big.Int).Add(prv, secret), temp_secp256k1.N)
}


// B_public_key = G * secret + A_public_key
// Used for implementing Type-2 determinitic keys
func DeriveNextPublic(prvx, prvy, secret *big.Int) (x, y *big.Int) {
	gsx, gsy := temp_secp256k1.ScalarBaseMult(secret.Bytes())
	x, y = temp_secp256k1.Add(prvx, prvy, gsx, gsy)
	return
}
