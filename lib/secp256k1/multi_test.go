package secp256k1

import (
	"encoding/hex"
	"math/rand"
	"testing"
)

/*
Test strings, that will cause failure
*/

//problem seckeys
var _test_seckey []string = []string{
	"08efb79385c9a8b0d1c6f5f6511be0c6f6c2902963d874a3a4bacc18802528d3",
	"78298d9ecdc0640c9ae6883201a53f4518055442642024d23c45858f45d0c3e6",
	"04e04fe65bfa6ded50a12769a3bd83d7351b2dbff08c9bac14662b23a3294b9e",
	"2f5141f1b75747996c5de77c911dae062d16ae48799052c04ead20ccd5afa113",
}

func RandBytes(n int) []byte {
	b := make([]byte, n, n)

	for i := 0; i < n; i++ {
		b[i] = byte(rand.Intn(256))
	}
	return b
}

//tests some keys that should work
func Test_Abnormal_Keys1(t *testing.T) {

	for i := 0; i < len(_test_seckey); i++ {

		seckey1, _ := hex.DecodeString(_test_seckey[i])

		pubkey1 := make([]byte, 33)

		ret := BaseMultiply(seckey1, pubkey1)

		if ret == false {
			t.Errorf("base multiplication fail")
		}
		//func BaseMultiply(k, out []byte) bool {

		var pubkey2 XY
		ret = pubkey2.ParsePubkey(pubkey1)
		if ret == false {
			t.Errorf("pubkey parse fail")
		}

		if pubkey2.IsValid() == false {
			t.Errorf("pubkey is not valid")
		}

	}
}

//tests random keys
func Test_Abnormal_Keys2(t *testing.T) {
	for i := 0; i < 64*1024; i++ {

		seckey1 := RandBytes(32)

		pubkey1 := make([]byte, 33)

		ret := BaseMultiply(seckey1, pubkey1)

		if ret == false {
			t.Error("base multiplication fail")
		}
		//func BaseMultiply(k, out []byte) bool {

		var pubkey2 XY
		ret = pubkey2.ParsePubkey(pubkey1)
		if ret == false {
			t.Error("pubkey parse fail")
		}

		if pubkey2.IsValid() == false {
			t.Error("pubkey is not valid for", hex.EncodeToString(seckey1))
		}
	}
}
