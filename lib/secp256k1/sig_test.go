package secp256k1

import (
	"crypto/rand"
	"encoding/hex"
	"strconv"
	"testing"
)

func TestPrintArch(t *testing.T) {
	println("Using field", FieldArch)
}

func TestSigRecover(t *testing.T) {
	var vs = [][6]string{
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
	var pubkey, exp XY
	var msg Number

	for i := range vs {
		sig.R.set_hex(vs[i][0])
		sig.S.set_hex(vs[i][1])
		msg.set_hex(vs[i][2])
		rid, _ := strconv.ParseInt(vs[i][3], 10, 32)
		exp.X.SetHex(vs[i][4])
		exp.Y.SetHex(vs[i][5])

		if sig.recover(&pubkey, &msg, int(rid)) {
			if !exp.X.Equals(&pubkey.X) {
				t.Error("X mismatch at vector", i)
			}
			if !exp.Y.Equals(&pubkey.Y) {
				t.Error("Y mismatch at vector", i)
			}
		} else {
			t.Error("sig.recover fialed")
		}
	}
}

func TestSigVerify(t *testing.T) {
	var msg Number
	var sig Signature
	var key XY

	msg.set_hex("3382219555ddbb5b00e0090f469e590ba1eae03c7f28ab937de330aa60294ed6")
	sig.R.set_hex("fe00e013c244062847045ae7eb73b03fca583e9aa5dbd030a8fd1c6dfcf11b10")
	sig.S.set_hex("7d0d04fed8fa1e93007468d5a9e134b0a7023b6d31db4e50942d43a250f4d07c")
	xy, _ := hex.DecodeString("040eaebcd1df2df853d66ce0e1b0fda07f67d1cabefde98514aad795b86a6ea66dbeb26b67d7a00e2447baeccc8a4cef7cd3cad67376ac1c5785aeebb4f6441c16")
	key.ParsePubkey(xy)
	if !sig.Verify(&key, &msg) {
		t.Error("sig.Verify 0")
	}

	msg.set_hex("D474CBF2203C1A55A411EEC4404AF2AFB2FE942C434B23EFE46E9F04DA8433CA")
	sig.R.set_hex("98F9D784BA6C5C77BB7323D044C0FC9F2B27BAA0A5B0718FE88596CC56681980")
	sig.S.set_hex("E3599D551029336A745B9FB01566624D870780F363356CEE1425ED67D1294480")
	key.X.SetHex("7d709f85a331813f9ae6046c56b3a42737abf4eb918b2e7afee285070e968b93")
	key.Y.SetHex("26150d1a63b342986c373977b00131950cb5fc194643cad6ea36b5157eba4602")
	if !sig.Verify(&key, &msg) {
		t.Error("sig.Verify 1")
	}

	msg.set_hex("2c43a883f4edc2b66c67a7a355b9312a565bb3d33bb854af36a06669e2028377")
	sig.R.set_hex("6b2fa9344462c958d4a674c2a42fbedf7d6159a5276eb658887e2e1b3915329b")
	sig.S.set_hex("eddc6ea7f190c14a0aa74e41519d88d2681314f011d253665f301425caf86b86")
	xy, _ = hex.DecodeString("02a60d70cfba37177d8239d018185d864b2bdd0caf5e175fd4454cc006fd2d75ac")
	key.ParsePubkey(xy)
	if !sig.Verify(&key, &msg) {
		t.Error("sig.Verify 2")
	}
}

func TestSigSign(t *testing.T) {
	var sec, msg, non Number
	var sig Signature
	var recid int
	sec.set_hex("73641C99F7719F57D8F4BEB11A303AFCD190243A51CED8782CA6D3DBE014D146")
	msg.set_hex("D474CBF2203C1A55A411EEC4404AF2AFB2FE942C434B23EFE46E9F04DA8433CA")
	non.set_hex("9E3CD9AB0F32911BFDE39AD155F527192CE5ED1F51447D63C4F154C118DA598E")
	res := sig.Sign(&sec, &msg, &non, &recid)
	if res != 1 {
		t.Error("res failed", res)
	}
	if FORCE_LOW_S {
		if recid != 0 {
			t.Error("recid failed", recid)
		}
	} else {
		if recid != 1 {
			t.Error("recid failed", recid)
		}
	}
	non.set_hex("98f9d784ba6c5c77bb7323d044c0fc9f2b27baa0a5b0718fe88596cc56681980")
	if sig.R.Cmp(&non.Int) != 0 {
		t.Error("R failed", sig.R.String())
	}
	if FORCE_LOW_S {
		non.set_hex("1ca662aaefd6cc958ba4604fea999db133a75bf34c13334dabac7124ff0cfcc1")
	} else {
		non.set_hex("E3599D551029336A745B9FB01566624D870780F363356CEE1425ED67D1294480")
	}
	if sig.S.Cmp(&non.Int) != 0 {
		t.Error("S failed", sig.S.String())
	}
}

func BenchmarkVerify(b *testing.B) {
	var msg Number
	var sig Signature
	var key XY
	msg.set_hex("D474CBF2203C1A55A411EEC4404AF2AFB2FE942C434B23EFE46E9F04DA8433CA")
	sig.R.set_hex("98F9D784BA6C5C77BB7323D044C0FC9F2B27BAA0A5B0718FE88596CC56681980")
	sig.S.set_hex("E3599D551029336A745B9FB01566624D870780F363356CEE1425ED67D1294480")
	key.X.SetHex("7d709f85a331813f9ae6046c56b3a42737abf4eb918b2e7afee285070e968b93")
	key.Y.SetHex("26150d1a63b342986c373977b00131950cb5fc194643cad6ea36b5157eba4602")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if !sig.Verify(&key, &msg) {
			b.Fatal("sig_verify failed")
		}
	}
}

func BenchmarkPrv2Pub(b *testing.B) {
	var prv [32]byte
	var pub [33]byte
	rand.Read(prv[:])
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		BaseMultiply(prv[:], pub[:])
	}
}

func BenchmarkSign(b *testing.B) {
	var sec, msg, non Number
	var sig Signature
	var recid int
	sec.set_hex("73641C99F7719F57D8F4BEB11A303AFCD190243A51CED8782CA6D3DBE014D146")
	msg.set_hex("D474CBF2203C1A55A411EEC4404AF2AFB2FE942C434B23EFE46E9F04DA8433CA")
	non.set_hex("9E3CD9AB0F32911BFDE39AD155F527192CE5ED1F51447D63C4F154C118DA598E")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sig.Sign(&sec, &msg, &non, &recid)
	}
}
