package secp256k1

import (
	"encoding/hex"
	"testing"
)

var ta = [][3]string{
	// [0]-pubScr, [1]-sigScript, [2]-unsignedTx
	{
		"040eaebcd1df2df853d66ce0e1b0fda07f67d1cabefde98514aad795b86a6ea66dbeb26b67d7a00e2447baeccc8a4cef7cd3cad67376ac1c5785aeebb4f6441c16",
		"3045022100fe00e013c244062847045ae7eb73b03fca583e9aa5dbd030a8fd1c6dfcf11b1002207d0d04fed8fa1e93007468d5a9e134b0a7023b6d31db4e50942d43a250f4d07c01",
		"3382219555ddbb5b00e0090f469e590ba1eae03c7f28ab937de330aa60294ed6",
	},
	{
		"020eaebcd1df2df853d66ce0e1b0fda07f67d1cabefde98514aad795b86a6ea66d",
		"3045022100fe00e013c244062847045ae7eb73b03fca583e9aa5dbd030a8fd1c6dfcf11b1002207d0d04fed8fa1e93007468d5a9e134b0a7023b6d31db4e50942d43a250f4d07c01",
		"3382219555ddbb5b00e0090f469e590ba1eae03c7f28ab937de330aa60294ed6",
	},
	{
		"0411db93e1dcdb8a016b49840f8c53bc1eb68a382e97b1482ecad7b148a6909a5cb2e0eaddfb84ccf9744464f82e160bfa9b8b64f9d4c03f999b8643f656b412a3",
		"304402204e45e16932b8af514961a1d3a1a25fdf3f4f7732e9d624c6c61548ab5fb8cd410220181522ec8eca07de4860a4acdd12909d831cc56cbbac4622082221a8768d1d0901",
		"7a05c6145f10101e9d6325494245adf1297d80f8f38d4d576d57cdba220bcb19",
	},
	{
		"0311db93e1dcdb8a016b49840f8c53bc1eb68a382e97b1482ecad7b148a6909a5c",
		"304402204e45e16932b8af514961a1d3a1a25fdf3f4f7732e9d624c6c61548ab5fb8cd410220181522ec8eca07de4860a4acdd12909d831cc56cbbac4622082221a8768d1d0901",
		"7a05c6145f10101e9d6325494245adf1297d80f8f38d4d576d57cdba220bcb19",
	},
	{
		"0428f42723f81c70664e200088437282d0e11ae0d4ae139f88bdeef1550471271692970342db8e3f9c6f0123fab9414f7865d2db90c24824da775f00e228b791fd",
		"3045022100d557da5d9bf886e0c3f98fd6d5d337487cd01d5b887498679a57e3d32bd5d0af0220153217b63a75c3145b14f58c64901675fe28dba2352c2fa9f2a1579c74a2de1701",
		"c22de395adbb0720941e009e8a4e488791b2e428af775432ed94d2c7ec8e421a",
	},
	{
		"0328f42723f81c70664e200088437282d0e11ae0d4ae139f88bdeef15504712716",
		"3045022100d557da5d9bf886e0c3f98fd6d5d337487cd01d5b887498679a57e3d32bd5d0af0220153217b63a75c3145b14f58c64901675fe28dba2352c2fa9f2a1579c74a2de1701",
		"c22de395adbb0720941e009e8a4e488791b2e428af775432ed94d2c7ec8e421a",
	},
	{
		"041f2a00036b3cbd1abe71dca54d406a1e9dd5d376bf125bb109726ff8f2662edcd848bd2c44a86a7772442095c7003248cc619bfec3ddb65130b0937f8311c787",
		"3045022100ec6eb6b2aa0580c8e75e8e316a78942c70f46dd175b23b704c0330ab34a86a34022067a73509df89072095a16dbf350cc5f1ca5906404a9275ebed8a4ba219627d6701",
		"7c8e7c2cb887682ed04dc82c9121e16f6d669ea3d57a2756785c5863d05d2e6a",
	},
	{
		"031f2a00036b3cbd1abe71dca54d406a1e9dd5d376bf125bb109726ff8f2662edc",
		"3045022100ec6eb6b2aa0580c8e75e8e316a78942c70f46dd175b23b704c0330ab34a86a34022067a73509df89072095a16dbf350cc5f1ca5906404a9275ebed8a4ba219627d6701",
		"7c8e7c2cb887682ed04dc82c9121e16f6d669ea3d57a2756785c5863d05d2e6a",
	},
	{
		"04ee90bfdd4e07eb1cfe9c6342479ca26c0827f84bfe1ab39e32fc3e94a0fe00e6f7d8cd895704e974978766dd0f9fad3c97b1a0f23684e93b400cc9022b7ae532",
		"3045022100fe1f6e2c2c2cbc916f9f9d16497df2f66a4834e5582d6da0ee0474731c4a27580220682bad9359cd946dc97bb07ea8fad48a36f9b61186d47c6798ccce7ba20cc22701",
		"baff983e6dfb1052918f982090aa932f56d9301d1de9a726d2e85d5f6bb75464",
	},
}

func TestVerify1(t *testing.T) {
	for i := range ta {
		pkey, _ := hex.DecodeString(ta[i][0])
		sign, _ := hex.DecodeString(ta[i][1])
		hasz, _ := hex.DecodeString(ta[i][2])

		res := ecdsa_verify(pkey, sign, hasz)
		if res != 1 {
			t.Fatal("Verify failed at", i)
		}

		hasz[0]++
		res = ecdsa_verify(pkey, sign, hasz)
		if res != 0 {
			t.Error("Verify not failed while it should", i)
		}
		res = ecdsa_verify(pkey[:1], sign, hasz)
		if res >= 0 {
			t.Error("Negative result expected", res, i)
		}
		res = ecdsa_verify(pkey, sign[:1], hasz)
		if res >= 0 {
			t.Error("Yet negative result expected", res, i)
		}
		res = ecdsa_verify(pkey, sign, hasz[:1])
		if res != 0 {
			t.Error("Zero expected", res, i)
		}
	}
}

func BenchmarkVerifyUncompressed(b *testing.B) {
	key, _ := hex.DecodeString("040eaebcd1df2df853d66ce0e1b0fda07f67d1cabefde98514aad795b86a6ea66dbeb26b67d7a00e2447baeccc8a4cef7cd3cad67376ac1c5785aeebb4f6441c16")
	sig, _ := hex.DecodeString("3045022100fe00e013c244062847045ae7eb73b03fca583e9aa5dbd030a8fd1c6dfcf11b1002207d0d04fed8fa1e93007468d5a9e134b0a7023b6d31db4e50942d43a250f4d07c01")
	msg, _ := hex.DecodeString("3382219555ddbb5b00e0090f469e590ba1eae03c7f28ab937de330aa60294ed6")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ecdsa_verify(key, sig, msg)
	}
}

func BenchmarkVerifyCompressed(b *testing.B) {
	key_compr, _ := hex.DecodeString("020eaebcd1df2df853d66ce0e1b0fda07f67d1cabefde98514aad795b86a6ea66d")
	sig, _ := hex.DecodeString("3045022100fe00e013c244062847045ae7eb73b03fca583e9aa5dbd030a8fd1c6dfcf11b1002207d0d04fed8fa1e93007468d5a9e134b0a7023b6d31db4e50942d43a250f4d07c01")
	msg, _ := hex.DecodeString("3382219555ddbb5b00e0090f469e590ba1eae03c7f28ab937de330aa60294ed6")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ecdsa_verify(key_compr, sig, msg)
	}
}

func TestECmult(t *testing.T) {
	var u1, u2 Number
	var pubkeyj, expres, pr XYZ

	pubkeyj.X.SetHex("0EAEBCD1DF2DF853D66CE0E1B0FDA07F67D1CABEFDE98514AAD795B86A6EA66D")
	pubkeyj.Y.SetHex("BEB26B67D7A00E2447BAECCC8A4CEF7CD3CAD67376AC1C5785AEEBB4F6441C16")
	pubkeyj.Z.SetHex("0000000000000000000000000000000000000000000000000000000000000001")

	u1.set_hex("B618EBA71EC03638693405C75FC1C9ABB1A74471BAAF1A3A8B9005821491C4B4")
	u2.set_hex("8554470195DE4678B06EDE9F9286545B51FF2D9AA756CE35A39011783563EA60")

	expres.X.SetHex("EB6752420B6BDB40A760AC26ADD7E7BBD080BF1DF6C0B009A0D310E4511BDF49")
	expres.Y.SetHex("8E8CEB84E1502FC536FFE67967BC44314270A0B38C79865FFED5A85D138DCA6B")
	expres.Z.SetHex("813925AF112AAB8243F8CCBADE4CC7F63DF387263028DE6E679232A73A7F3C31")

	pubkeyj.ECmult(&pr, &u2, &u1)
	if !pr.Equals(&expres) {
		t.Error("ECmult failed")
		pr.Print("got")
		expres.Print("exp")
	}
}

type wnafvec struct {
	inp string
	exp []int
	w   uint
}

func TestWNAF(t *testing.T) {
	var wnaf [129]int
	var testvcs = []wnafvec{
		{
			"3271156f58b59bd7aa542ca6972c1910", WINDOW_A,
			[]int{0, 0, 0, 0, -15, 0, 0, 0, 0, 13, 0, 0, 0, 0, 0, 0, 0, 0, 11, 0, 0, 0, 0, 0, -9, 0, 0, 0, 0, -11, 0, 0, 0, 0, 0, -11, 0, 0, 0, 0, 13, 0, 0, 0, 0, 1, 0, 0, 0, 0, -11, 0, 0, 0, 0, -11, 0, 0, 0, 0, -5, 0, 0, 0, 0, 0, 0, -5, 0, 0, 0, 0, 0, 0, 7, 0, 0, 0, 0, 11, 0, 0, 0, 0, 11, 0, 0, 0, 0, 0, 0, 11, 0, 0, 0, 0, 15, 0, 0, 0, 0, 11, 0, 0, 0, 0, 5, 0, 0, 0, 0, 0, -15, 0, 0, 0, 0, 0, 0, 5, 0, 0, 0, 0, 3},
		},
		{
			"0a8a5afcb465a43b8277801311860430", WINDOW_A,
			[]int{0, 0, 0, 0, 3, 0, 0, 0, 0, 0, 1, 0, 0, 0, 0, 0, 0, 3, 0, 0, 0, 0, 0, 3, 0, 0, 0, 0, -15, 0, 0, 0, 0, 0, 5, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 15, 0, 0, 0, 0, 7, 0, 0, 0, 0, 1, 0, 0, 0, 0, 0, -9, 0, 0, 0, 0, 0, 0, -15, 0, 0, 0, 0, -11, 0, 0, 0, 0, 0, -13, 0, 0, 0, 0, 0, 9, 0, 0, 0, 0, 11, 0, 0, 0, 0, 0, -1, 0, 0, 0, 0, 0, -5, 0, 0, 0, 0, -13, 0, 0, 0, 0, 3, 0, 0, 0, 0, -11, 0, 0, 0, 0, 1},
		},
		{
			"b1a74471baaf1a3a8b9005821491c4b4", WINDOW_G,
			[]int{0, 0, -3795, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 2633, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 705, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, -5959, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1679, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, -1361, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 4551, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1693, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 11},
		},
		{
			"b618eba71ec03638693405c75fc1c9ab", WINDOW_G,
			[]int{2475, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, -249, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, -4549, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, -6527, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 7221, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, -8165, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, -6369, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, -7249, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1457},
		},
	}
	for idx := range testvcs {
		var xxx Number
		xxx.set_hex(testvcs[idx].inp)
		bits := ecmult_wnaf(wnaf[:], &xxx, testvcs[idx].w)
		if bits != len(testvcs[idx].exp) {
			t.Error("Bad bits at idx", idx)
		}
		for i := range testvcs[idx].exp {
			if wnaf[i] != testvcs[idx].exp[i] {
				t.Error("Bad val at idx", idx, i)
			}
		}
	}
}

func TestPrecompileGej(t *testing.T) {
	var exp, a XYZ

	a.X.SetHex("0eaebcd1df2df853d66ce0e1b0fda07f67d1cabefde98514aad795b86a6ea66d")
	a.Y.SetHex("beb26b67d7a00e2447baeccc8a4cef7cd3cad67376ac1c5785aeebb4f6441c16")
	a.Z.SetHex("01")
	exp.X.SetHex("ce5dcac5e26ab63868ead1440f359aff29d7ffade62abe801bca97b471bcd416")
	exp.Y.SetHex("0cc6f63793a207751d507aa4be629f0776441e4873548095bd6d39d34ce8a9d7")
	exp.Z.SetHex("122927e4908740d51df1f03dc921c00fef68c542e7f28aa270862619cf971815")
	pre := a.precomp(WINDOW_A)
	if len(pre) != 8 {
		t.Error("Bad result length")
	}
	if !pre[7].Equals(&exp) {
		t.Error("Unexpcted value")
	}

	a.X.SetHex("a45720c272cfa1f77f64be8a404a7d3149bd5410f9a173353f6eb75a5085ba98")
	a.Y.SetHex("beb26b67d7a00e2447baeccc8a4cef7cd3cad67376ac1c5785aeebb4f6441c16")
	a.Z.SetHex("01")
	exp.X.SetHex("ce5dcac5e26ab63868ead1440f359aff29d7ffade62abe801bca97b471bcd416")
	exp.Y.SetHex("0cc6f63793a207751d507aa4be629f0776441e4873548095bd6d39d34ce8a9d7")
	exp.Z.SetHex("49f0fb9f1840e7a58d485c6cc394e597e521bf7d4598be2b367c27326949e507")
	pre = a.precomp(WINDOW_A)
	if len(pre) != 8 {
		t.Error("Bad result length")
	}
	if !pre[7].Equals(&exp) {
		t.Error("Unexpcted value")
	}
}

func TestMultGen(t *testing.T) {
	var nonce Number
	var ex, ey, ez Field
	var r XYZ
	nonce.set_hex("9E3CD9AB0F32911BFDE39AD155F527192CE5ED1F51447D63C4F154C118DA598E")
	ECmultGen(&r, &nonce)
	ex.SetHex("02D1BF36D37ACD68E4DD00DB3A707FD176A37E42F81AEF9386924032D3428FF0")
	ey.SetHex("FD52E285D33EC835230EA69F89D9C38673BD5B995716A4063C893AF02F938454")
	ez.SetHex("4C6ACE7C8C062A1E046F66FD8E3981DC4E8E844ED856B5415C62047129268C1B")
	r.X.Normalize()
	r.Y.Normalize()
	r.Z.Normalize()
	if !ex.Equals(&r.X) {
		t.Error("Bad X")
	}
	if !ey.Equals(&r.Y) {
		t.Error("Bad Y")
	}
	if !ez.Equals(&r.Z) {
		t.Error("Bad Y")
	}
}
