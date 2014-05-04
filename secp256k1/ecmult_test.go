package secp256k1

import (
	"testing"
)


func TestECmult(t *testing.T) {
	var u1, u2 Number
	var pubkeyj, expres, pr XYZ_t

	pubkeyj.x.SetHex("0EAEBCD1DF2DF853D66CE0E1B0FDA07F67D1CABEFDE98514AAD795B86A6EA66D")
	pubkeyj.y.SetHex("BEB26B67D7A00E2447BAECCC8A4CEF7CD3CAD67376AC1C5785AEEBB4F6441C16")
	pubkeyj.z.SetHex("0000000000000000000000000000000000000000000000000000000000000001")

	u1.SetHex("B618EBA71EC03638693405C75FC1C9ABB1A74471BAAF1A3A8B9005821491C4B4")
	u2.SetHex("8554470195DE4678B06EDE9F9286545B51FF2D9AA756CE35A39011783563EA60")

	expres.x.SetHex("EB6752420B6BDB40A760AC26ADD7E7BBD080BF1DF6C0B009A0D310E4511BDF49")
	expres.y.SetHex("8E8CEB84E1502FC536FFE67967BC44314270A0B38C79865FFED5A85D138DCA6B")
	expres.z.SetHex("813925AF112AAB8243F8CCBADE4CC7F63DF387263028DE6E679232A73A7F3C31")

	pubkeyj.ecmult(&pr, &u2, &u1)
	if !pr.Equals(&expres) {
		t.Error("ecmult failed")
		pr.Print("got")
		expres.Print("exp")
	}
}

type wnafvec struct {
	inp string
	w uint
	exp []int
}

func TestWNAF(t *testing.T) {
	var wnaf [129]int
	var testvcs = []wnafvec {
		{
			"3271156f58b59bd7aa542ca6972c1910", WINDOW_A,
			[]int{0,0,0,0,-15,0,0,0,0,13,0,0,0,0,0,0,0,0,11,0,0,0,0,0,-9,0,0,0,0,-11,0,0,0,0,0,-11,0,0,0,0,13,0,0,0,0,1,0,0,0,0,-11,0,0,0,0,-11,0,0,0,0,-5,0,0,0,0,0,0,-5,0,0,0,0,0,0,7,0,0,0,0,11,0,0,0,0,11,0,0,0,0,0,0,11,0,0,0,0,15,0,0,0,0,11,0,0,0,0,5,0,0,0,0,0,-15,0,0,0,0,0,0,5,0,0,0,0,3},
		},
		{
			"0a8a5afcb465a43b8277801311860430", WINDOW_A,
			[]int{0,0,0,0,3,0,0,0,0,0,1,0,0,0,0,0,0,3,0,0,0,0,0,3,0,0,0,0,-15,0,0,0,0,0,5,0,0,0,0,0,0,0,0,0,0,0,0,15,0,0,0,0,7,0,0,0,0,1,0,0,0,0,0,-9,0,0,0,0,0,0,-15,0,0,0,0,-11,0,0,0,0,0,-13,0,0,0,0,0,9,0,0,0,0,11,0,0,0,0,0,-1,0,0,0,0,0,-5,0,0,0,0,-13,0,0,0,0,3,0,0,0,0,-11,0,0,0,0,1},
		},
		{
			"b1a74471baaf1a3a8b9005821491c4b4", WINDOW_G,
			[]int{0,0,-3795,0,0,0,0,0,0,0,0,0,0,0,0,0,0,2633,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,705,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,-5959,0,0,0,0,0,0,0,0,0,0,0,0,0,1679,0,0,0,0,0,0,0,0,0,0,0,0,0,-1361,0,0,0,0,0,0,0,0,0,0,0,0,0,4551,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,1693,0,0,0,0,0,0,0,0,0,0,0,0,0,11},
		},
		{
			"b618eba71ec03638693405c75fc1c9ab", WINDOW_G,
			[]int{2475,0,0,0,0,0,0,0,0,0,0,0,0,0,-249,0,0,0,0,0,0,0,0,0,0,0,0,0,0,-4549,0,0,0,0,0,0,0,0,0,0,0,0,0,-6527,0,0,0,0,0,0,0,0,0,0,0,0,0,7221,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,-8165,0,0,0,0,0,0,0,0,0,0,0,0,0,0,-6369,0,0,0,0,0,0,0,0,0,0,0,0,0,-7249,0,0,0,0,0,0,0,0,0,0,0,0,0,0,1457},
		},
	}
	for idx := range testvcs {
		var xxx Number
		xxx.SetHex(testvcs[idx].inp)
		bits := ecmult_wnaf(wnaf[:], &xxx, testvcs[idx].w)
		if bits != len(testvcs[idx].exp) {
			t.Error("Bad bits at idx", idx)
		}
		for i := range testvcs[idx].exp {
			if wnaf[i]!=testvcs[idx].exp[i] {
				t.Error("Bad val at idx", idx, i)
			}
		}
	}
}


func TestPrecompileGej(t *testing.T) {
	var exp, a XYZ_t

	a.x.SetHex("0eaebcd1df2df853d66ce0e1b0fda07f67d1cabefde98514aad795b86a6ea66d")
	a.y.SetHex("beb26b67d7a00e2447baeccc8a4cef7cd3cad67376ac1c5785aeebb4f6441c16")
	a.z.SetHex("01")
	exp.x.SetHex("ce5dcac5e26ab63868ead1440f359aff29d7ffade62abe801bca97b471bcd416")
	exp.y.SetHex("0cc6f63793a207751d507aa4be629f0776441e4873548095bd6d39d34ce8a9d7")
	exp.z.SetHex("122927e4908740d51df1f03dc921c00fef68c542e7f28aa270862619cf971815")
	pre := a.precomp(WINDOW_A)
	if len(pre)!=8 {
		t.Error("Bad result length")
	}
	if !pre[7].Equals(&exp) {
		t.Error("Unexpcted value")
	}

	a.x.SetHex("a45720c272cfa1f77f64be8a404a7d3149bd5410f9a173353f6eb75a5085ba98")
	a.y.SetHex("beb26b67d7a00e2447baeccc8a4cef7cd3cad67376ac1c5785aeebb4f6441c16")
	a.z.SetHex("01")
	exp.x.SetHex("ce5dcac5e26ab63868ead1440f359aff29d7ffade62abe801bca97b471bcd416")
	exp.y.SetHex("0cc6f63793a207751d507aa4be629f0776441e4873548095bd6d39d34ce8a9d7")
	exp.z.SetHex("49f0fb9f1840e7a58d485c6cc394e597e521bf7d4598be2b367c27326949e507")
	pre = a.precomp(WINDOW_A)
	if len(pre)!=8 {
		t.Error("Bad result length")
	}
	if !pre[7].Equals(&exp) {
		t.Error("Unexpcted value")
	}
}


func TestMultGen(t *testing.T) {
	var nonce  Number
	var ex, ey, ez Fe_t
	var r XYZ_t
	nonce.SetHex("9E3CD9AB0F32911BFDE39AD155F527192CE5ED1F51447D63C4F154C118DA598E")
	ecmult_gen(&r, &nonce)
	ex.SetHex("02D1BF36D37ACD68E4DD00DB3A707FD176A37E42F81AEF9386924032D3428FF0")
	ey.SetHex("FD52E285D33EC835230EA69F89D9C38673BD5B995716A4063C893AF02F938454")
	ez.SetHex("4C6ACE7C8C062A1E046F66FD8E3981DC4E8E844ED856B5415C62047129268C1B")
	r.x.Normalize()
	r.y.Normalize()
	r.z.Normalize()
	if !ex.Equals(&r.x) {
		t.Error("Bad X")
	}
	if !ey.Equals(&r.y) {
		t.Error("Bad Y")
	}
	if !ez.Equals(&r.z) {
		t.Error("Bad Y")
	}
}
