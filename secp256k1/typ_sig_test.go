package secp256k1

import (
	"strconv"
	"testing"
)

func TestSigRecover(t *testing.T) {
	var vs = [][6]string {
		{
			"6028b9e3a31c9e725fcbd7d5d16736aaaafcc9bf157dfb4be62bcbcf0969d488",
			"036d4a36fa235b8f9f815aa6f5457a607f956a71a035bf0970d8578bf218bb5a",
			"9cff3da1a4f86caf3683f865232c64992b5ed002af42b321b8d8a48420680487",
			"0",
			"56dc5df245955302893d8dda0677cc9865d8011bc678c7803a18b5f6faafec08",
			"54b5fbdcd8fac6468dac2de88fadce6414f5f3afbb103753e25161bef77705a6",
		},
		{
			"b470e02f834a3aaafa27bd2b49e07269e962a51410f364e9e195c31351a05e50",
			"560978aed76de9d5d781f87ed2068832ed545f2b21bf040654a2daff694c8b09",
			"9ce428d58e8e4caf619dc6fc7b2c2c28f0561654d1f80f322c038ad5e67ff8a6",
			"1",
			"15b7e7d00f024bffcd2e47524bb7b7d3a6b251e23a3a43191ed7f0a418d9a578",
			"bf29a25e2d1f32c5afb18b41ae60112723278a8af31275965a6ec1d95334e840",
		},
	}

	var sig Signature
	var pubkey, exp ge_t
	var msg Number

	for i := range vs {
		sig.r.SetHex(vs[i][0])
		sig.s.SetHex(vs[i][1])
		msg.SetHex(vs[i][2])
		rid, _ := strconv.ParseInt(vs[i][3], 10, 32)
		exp.x.SetHex(vs[i][4])
		exp.y.SetHex(vs[i][5])

		if sig.recover(&pubkey, &msg, int(rid)) {
			if !exp.x.Equals(&pubkey.x) {
				t.Error("x mismatch at vector", i)
			}
			if !exp.y.Equals(&pubkey.y) {
				t.Error("y mismatch at vector", i)
			}
		} else {
			t.Error("sig.recover fialed")
		}
	}
}

func TestSign(t *testing.T) {
	var sec, msg, non Number
	var sig Signature
	var recid int
	sec.SetHex("73641C99F7719F57D8F4BEB11A303AFCD190243A51CED8782CA6D3DBE014D146")
	msg.SetHex("D474CBF2203C1A55A411EEC4404AF2AFB2FE942C434B23EFE46E9F04DA8433CA")
	non.SetHex("9E3CD9AB0F32911BFDE39AD155F527192CE5ED1F51447D63C4F154C118DA598E")
	res := sig.Sign(&sec, &msg, &non, &recid)
	if res != 1 {
		t.Error("res failed", res)
	}
	if recid != 1 {
		t.Error("recid failed", recid)
	}
	non.SetHex("98f9d784ba6c5c77bb7323d044c0fc9f2b27baa0a5b0718fe88596cc56681980")
	if sig.r.Cmp(&non.Int)!=0 {
		t.Error("R failed", sig.r.String())
	}
	non.SetHex("E3599D551029336A745B9FB01566624D870780F363356CEE1425ED67D1294480")
	if sig.s.Cmp(&non.Int)!=0 {
		t.Error("S failed", sig.s.String())
	}
}


func BenchmarkVerify(b *testing.B) {
	var msg Number
	var sig Signature
	var key ge_t
	msg.SetHex("D474CBF2203C1A55A411EEC4404AF2AFB2FE942C434B23EFE46E9F04DA8433CA")
	sig.r.SetHex("98F9D784BA6C5C77BB7323D044C0FC9F2B27BAA0A5B0718FE88596CC56681980")
	sig.s.SetHex("E3599D551029336A745B9FB01566624D870780F363356CEE1425ED67D1294480")
	key.x.SetHex("7d709f85a331813f9ae6046c56b3a42737abf4eb918b2e7afee285070e968b93")
	key.y.SetHex("26150d1a63b342986c373977b00131950cb5fc194643cad6ea36b5157eba4602")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if !sig.sig_verify(&key, &msg) {
			b.Fatal("sig_verify failed")
		}
	}
}


func BenchmarkSign(b *testing.B) {
	var sec, msg, non Number
	var sig Signature
	var recid int
	sec.SetHex("73641C99F7719F57D8F4BEB11A303AFCD190243A51CED8782CA6D3DBE014D146")
	msg.SetHex("D474CBF2203C1A55A411EEC4404AF2AFB2FE942C434B23EFE46E9F04DA8433CA")
	non.SetHex("9E3CD9AB0F32911BFDE39AD155F527192CE5ED1F51447D63C4F154C118DA598E")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sig.Sign(&sec, &msg, &non, &recid)
	}
}
