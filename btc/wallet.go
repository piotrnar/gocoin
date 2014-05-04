package btc

import (
	"fmt"
	"errors"
	"math/big"
	"crypto/rand"
	"crypto/ecdsa"
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
	const TestMessage = "Just some test message..."
	hash := Sha2Sum([]byte(TestMessage))

	pub_key, e := NewPublicKey(publ)
	if e != nil {
		return e
	}

	var key ecdsa.PrivateKey
	key.D = new(big.Int).SetBytes(priv)
	key.PublicKey = pub_key.PublicKey

	if key.D.Cmp(big.NewInt(0)) == 0 {
		return errors.New("pubkey value is zero")
	}

	if key.D.Cmp(temp_secp256k1.N) != -1 {
		return errors.New("pubkey value is too big")
	}

	r, s, err := ecdsa.Sign(rand.Reader, &key, hash[:])
	if err != nil {
		return errors.New("ecdsa.Sign failed: " + err.Error())
	}

	ok := ecdsa.Verify(&key.PublicKey, hash[:], r, s)
	if !ok {
		return errors.New("ecdsa.Sign Verify")
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
