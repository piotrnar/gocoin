package main

import (
	"os"
	"math/big"
	"crypto/rand"
	"crypto/ecdsa"
	"encoding/hex"
	"github.com/piotrnar/gocoin/btc"
)

var curv *btc.BitCurve = btc.S256()

func xy2pk(x, y *big.Int) (res []byte) {
	res = make([]byte, 65)
	res[0] = 4
	xb := x.Bytes()
	yb := y.Bytes()
	copy(res[1+32-len(xb):], xb)
	copy(res[33+32-len(yb):], yb)
	return
}


// Verify the secret key's range and al if a test message signed with it verifies OK
func verify_key(priv []byte, publ []byte) bool {
	const TestMessage = "Just some test message..."
	hash := btc.Sha2Sum([]byte(TestMessage))

	pub_key, e := btc.NewPublicKey(publ)
	if e != nil {
		println("NewPublicKey:", e.Error())
		os.Exit(1)
	}

	var key ecdsa.PrivateKey
	key.D = new(big.Int).SetBytes(priv)
	key.PublicKey = pub_key.PublicKey

	if key.D.Cmp(big.NewInt(0)) == 0 {
		println("pubkey value is zero")
		return false
	}

	if key.D.Cmp(curv.N) != -1 {
		println("pubkey value is too big", hex.EncodeToString(publ))
		return false
	}

	r, s, err := ecdsa.Sign(rand.Reader, &key, hash[:])
	if err != nil {
		println("ecdsa.Sign:", err.Error())
		return false
	}

	ok := ecdsa.Verify(&key.PublicKey, hash[:], r, s)
	if !ok {
		println("The key pair does not verify!")
		return false
	}
	return true
}


//B_private_key = ( A_private_key + secret ) % N
func derive_private_key(prv, secret *big.Int) (res *big.Int) {
	res = new(big.Int).Add(prv, secret)
	res = new(big.Int).Mod(res, curv.N)
	return
}


//B_public_key = G * secret + A_public_key
func derive_public_key(prvx, prvy, secret *big.Int) (x, y *big.Int) {
	bspX, bspY := curv.ScalarBaseMult(secret.Bytes())
	x, y = curv.Add(prvx, prvy, bspX, bspY)
	return
}

func main() {
	var buf [32]byte
	rand.Read(buf[:])
	secret := new(big.Int).SetBytes(buf[:])

	rand.Read(buf[:])
	A_private_key := new(big.Int).SetBytes(buf[:])

	println("p", hex.EncodeToString(A_private_key.Bytes()))
	println("q", hex.EncodeToString(secret.Bytes()))

	x, y := curv.ScalarBaseMult(A_private_key.Bytes())
	println("x", hex.EncodeToString(x.Bytes()))
	println("y", hex.EncodeToString(y.Bytes()))
	println(hex.EncodeToString(xy2pk(x, y)))

	var i int
	for i=0; i<100; i++ {
		private_key_B := derive_private_key(A_private_key, secret)
		bX, bY := derive_public_key(x, y, secret)

		// verify the public key matching the private key
		xB, yB := curv.ScalarBaseMult(private_key_B.Bytes())
		if bX.Cmp(xB)!=0 {
			println(i, "x error", hex.EncodeToString(bX.Bytes()))
			return
		}
		if bY.Cmp(yB)!=0 {
			println("y error", hex.EncodeToString(bY.Bytes()))
			return
		}

		// make sure that you can sign and verify with it
		if !verify_key(A_private_key.Bytes(), xy2pk(x,y)) {
			println(i, "verify key failed")
		}

		A_private_key = private_key_B
		x, y = bX, bY
	}
	println(i, "deterministic type 2 keys tested OK")
}
