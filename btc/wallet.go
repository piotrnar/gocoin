package btc

import (
	"errors"
	"math/big"
	"github.com/piotrnar/gocoin/secp256k1"
)


// Get ECDSA public key in bitcoin protocol format, from the give private key
func PublicFromPrivate(priv_key []byte, compressed bool) (res []byte) {
	if compressed {
		res = make([]byte, 33)
	} else {
		res = make([]byte, 65)
	}

	if !secp256k1.BaseMultiply(priv_key, res) {
		res = nil
	}
	return
}


// Verify the secret key's range and if a test message signed with it verifies OK
// Returns nil if everything looks OK
func VerifyKeyPair(priv []byte, publ []byte) error {
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

	r, s, e := EcdsaSign(priv, hash[:])
	if e != nil {
		return errors.New("EcdsaSign failed: " + e.Error())
	}

	sig.R.Set(r)
	sig.S.Set(s)
	ok := EcdsaVerify(publ, sig.Bytes(), hash[:])
	if !ok {
		return errors.New("EcdsaVerify failed")
	}
	return nil
}

// B_private_key = ( A_private_key + secret ) % N
// Used for implementing Type-2 determinitic keys
func DeriveNextPrivate(p, s []byte) []byte {
	var prv, secret big.Int
	prv.SetBytes(p)
	secret.SetBytes(s)
	return new(big.Int).Mod(new(big.Int).Add(&prv, &secret), &secp256k1.TheCurve.Order.Int).Bytes()
}


// B_public_key = G * secret + A_public_key
// Used for implementing Type-2 determinitic keys
func DeriveNextPublic(public, secret []byte) (out []byte) {
	out = make([]byte, len(public))
	secp256k1.BaseMultiplyAdd(public, secret, out)
	return
}
