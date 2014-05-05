package btc

import (
	"bytes"
	"testing"
	"math/big"
	"crypto/rand"
	"encoding/hex"
)

// convert x,y pubkey to 04... stuff
func xy2pk(x, y *big.Int) (res []byte) {
	res = make([]byte, 65)
	res[0] = 4
	xb := x.Bytes()
	yb := y.Bytes()
	copy(res[1+32-len(xb):], xb)
	copy(res[33+32-len(yb):], yb)
	return
}

func TestGetPublic(t *testing.T) {
	prv, _ := hex.DecodeString("bb87a5e3e786ecd05f4901ef7ef32726570bfd176ada37a31ef2861db2834d7e")
	pub, _ := hex.DecodeString("02a60d70cfba37177d8239d018185d864b2bdd0caf5e175fd4454cc006fd2d75ac")
	pk, _ := PublicFromPrivate(prv, true)
	if !bytes.Equal(pub, pk) {
		t.Error("PublicFromPrivate failed")
	}
}


func _TestDeterministicWalletType2(t *testing.T) {
	var buf [32]byte
	rand.Read(buf[:])
	secret := new(big.Int).SetBytes(buf[:])

	rand.Read(buf[:])
	A_private_key := new(big.Int).SetBytes(buf[:])

	//println("p", hex.EncodeToString(A_private_key.Bytes()))
	//println("q", hex.EncodeToString(secret.Bytes()))

	x, y := temp_secp256k1.ScalarBaseMult(A_private_key.Bytes())
	//println("x", hex.EncodeToString(x.Bytes()))
	//println("y", hex.EncodeToString(y.Bytes()))
	//println(hex.EncodeToString(xy2pk(x, y)))

	var i int
	for i=0; i<50; i++ {
		private_key_B := DeriveNextPrivate(A_private_key, secret)
		bX, bY := DeriveNextPublic(x, y, secret)

		// verify the public key matching the private key
		xB, yB := temp_secp256k1.ScalarBaseMult(private_key_B.Bytes())
		if bX.Cmp(xB)!=0 {
			t.Error(i, "x error", hex.EncodeToString(bX.Bytes()))
		}
		if bY.Cmp(yB)!=0 {
			t.Error(i, "y error", hex.EncodeToString(bY.Bytes()))
		}

		// make sure that you can sign and verify with it

		if e := VerifyKeyPair(A_private_key.Bytes(), xy2pk(x,y)); e!=nil {
			t.Error(i, "verify key failed", e.Error())
		}

		A_private_key = private_key_B
		x, y = bX, bY
	}
}
