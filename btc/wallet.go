package btc

import (
	"errors"
	"math/big"
	"github.com/piotrnar/gocoin/secp256k1"
)


// Get ECDSA public key in bitcoin protocol format, from the give private key
func PublicFromPrivate(priv_key []byte, compressed bool) (res []byte, e error) {
	if compressed {
		res = make([]byte, 33)
	} else {
		res = make([]byte, 65)
	}

	if !secp256k1.BaseMultiply(priv_key, res) {
		e = errors.New("BaseMultiply failed")
		res = nil
		return
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
func DeriveNextPrivate(p, s []byte) []byte {
	var prv, secret big.Int
	prv.SetBytes(p)
	secret.SetBytes(s)
	return new(big.Int).Mod(new(big.Int).Add(&prv, &secret), temp_secp256k1.N).Bytes()
}


// B_public_key = G * secret + A_public_key
// Used for implementing Type-2 determinitic keys
func DeriveNextPublic(public, secret []byte) (out []byte) {
	var pub secp256k1.XY
	if !pub.ParsePubkey(public) {
		return
	}
	pub.Multi(secret)
	pub.AddXY(&secp256k1.TheCurve.G)
	out = make([]byte, len(public))
	pub.GetPublicKey(out)
	return
}


func DeriveNextPublicOld(prvx, prvy, secret *big.Int) (x, y *big.Int) {
	gsx, gsy := temp_secp256k1.ScalarBaseMult(secret.Bytes())
	x, y = temp_secp256k1.Add(prvx, prvy, gsx, gsy)
	return
}
