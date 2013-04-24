package btc

import (
	"testing"
	"encoding/hex"
)

var ta = [][3]string{
	{ // [0]-pubScr, [1]-sigScript, [2]-unsignedTx
		"040eaebcd1df2df853d66ce0e1b0fda07f67d1cabefde98514aad795b86a6ea66dbeb26b67d7a00e2447baeccc8a4cef7cd3cad67376ac1c5785aeebb4f6441c16",
		"3045022100fe00e013c244062847045ae7eb73b03fca583e9aa5dbd030a8fd1c6dfcf11b1002207d0d04fed8fa1e93007468d5a9e134b0a7023b6d31db4e50942d43a250f4d07c01",
		"01000000014d276db8e3a547cc3eaff4051d0d158da21724634d7c67c51129fa403dded5de010000001976a914718950ac3039e53fbd6eb0213de333b689a1ca1288acffffffff02a8d39b0f000000001976a914db641fc6dff262fe2504725f2c4c1852b18ffe3588ace693f205000000001976a9141321c4f37c5b2be510c1c7725a83e561ad27876b88ac00000000",
	},
	{
		"0411db93e1dcdb8a016b49840f8c53bc1eb68a382e97b1482ecad7b148a6909a5cb2e0eaddfb84ccf9744464f82e160bfa9b8b64f9d4c03f999b8643f656b412a3",
		"304402204e45e16932b8af514961a1d3a1a25fdf3f4f7732e9d624c6c61548ab5fb8cd410220181522ec8eca07de4860a4acdd12909d831cc56cbbac4622082221a8768d1d0901",
		"0100000001c997a5e56e104102fa209c6a852dd90660a20b2d9c352423edce25857fcd37040000000043410411db93e1dcdb8a016b49840f8c53bc1eb68a382e97b1482ecad7b148a6909a5cb2e0eaddfb84ccf9744464f82e160bfa9b8b64f9d4c03f999b8643f656b412a3acffffffff0200ca9a3b00000000434104ae1a62fe09c5f51b13905f07f06b99a2f7159b2225f374cd378d71302fa28414e7aab37397f554a7df5f142c21c1b7303b8a0626f1baded5c72a704f7e6cd84cac00286bee0000000043410411db93e1dcdb8a016b49840f8c53bc1eb68a382e97b1482ecad7b148a6909a5cb2e0eaddfb84ccf9744464f82e160bfa9b8b64f9d4c03f999b8643f656b412a3ac00000000",
	},
}

func TestVerify(t *testing.T) {
	for i := range ta {
		b, e := hex.DecodeString(ta[i][0])
		if e != nil {
			panic(e.Error())
		}
		key, e := NewPublicKey(b[:])
		if e != nil {
			panic(e.Error())
		}

		// signature script
		b, e = hex.DecodeString(ta[i][1])
		if e != nil {
			panic(e.Error())
		}
		sig, e := NewSignature(b[:])
		if e != nil {
			panic(e.Error())
		}

		// hash of the message
		b, e = hex.DecodeString(ta[i][2] + "01000000")
		if e != nil {
			panic(e.Error())
		}
		h := NewSha2Hash(b[:])
		
		ok := key.Verify(h.Hash[:], sig)
		if !ok {
			t.Error("Test vector failed", i)
		}
	}
}


func BenchmarkNewSignature(b *testing.B) {
	ptr, _ := hex.DecodeString(ta[0][1])
	for i := 0; i < b.N; i++ {
		NewSignature(ptr[:])
	}
}


func BenchmarkVerify(b *testing.B) {
	ptr, _ := hex.DecodeString(ta[0][0])
	key, _ := NewPublicKey(ptr[:])
	ptr, _ = hex.DecodeString(ta[0][1])
	sig, _ := NewSignature(ptr[:])
	ptr, _ = hex.DecodeString(ta[0][2] + "01000000")
	h := NewSha2Hash(ptr[:])
	for i := 0; i < b.N; i++ {
		key.Verify(h.Hash[:], sig)
	}
}

