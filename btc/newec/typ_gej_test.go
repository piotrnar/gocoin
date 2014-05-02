package newec

import (
	"testing"
)


func _TestGejDouble(t *testing.T) {
	var a, a_exp, r gej_t
	a.x.set_hex("79be667ef9dcbbac55a06295ce870b07029bfcdb2dce28d959f2815b16f81798")
	a.y.set_hex("483ada7726a3c4655da4fbfc0e1108a8fd17b448a68554199c47d08ffb10d4b8")
	a.z.set_hex("01")
	a_exp.x.set_hex("7D152C041EA8E1DC2191843D1FA9DB55B68F88FEF695E2C791D40444B365AFC2")
	a_exp.y.set_hex("56915849F52CC8F76F5FD7E4BF60DB4A43BF633E1B1383F85FE89164BFADCBDB")
	a_exp.z.set_hex("9075B4EE4D4788CABB49F7F81C221151FA2F68914D0AA833388FA11FF621A970")

	a.double(&r)
	if !r.equal(&a_exp) {
		t.Error("gej.double failed")
	}
}

func TestGejMulLambda(t *testing.T) {
	var a, a_exp gej_t
	a.x.set_hex("0eaebcd1df2df853d66ce0e1b0fda07f67d1cabefde98514aad795b86a6ea66d")
	a.y.set_hex("beb26b67d7a00e2447baeccc8a4cef7cd3cad67376ac1c5785aeebb4f6441c16")
	a.z.set_hex("01")
	a_exp.x.set_hex("a45720c272cfa1f77f64be8a404a7d3149bd5410f9a173353f6eb75a5085ba98")
	a_exp.y.set_hex("beb26b67d7a00e2447baeccc8a4cef7cd3cad67376ac1c5785aeebb4f6441c16")
	a_exp.z.set_hex("01")
	a_lam := a
	a_lam.mul_lambda(&a_lam)
	if !a_lam.equal(&a_exp) {
		t.Error("mul_lambda failed")
	}
}

func TestGejGetX(t *testing.T) {
	var a gej_t
	var x, exp fe_t
	a.x.set_hex("EB6752420B6BDB40A760AC26ADD7E7BBD080BF1DF6C0B009A0D310E4511BDF49")
	a.y.set_hex("8E8CEB84E1502FC536FFE67967BC44314270A0B38C79865FFED5A85D138DCA6B")
	a.z.set_hex("813925AF112AAB8243F8CCBADE4CC7F63DF387263028DE6E679232A73A7F3C31")

	exp.set_hex("fe00e013c244062847045ae7eb73b03fca583e9aa5dbd030a8fd1c6dfcf11b10")
	a.get_x(&x)
	if !x.equal(&exp) {
		t.Error("get.get_x() fail")
	}
}


func TestGejTwice(t *testing.T) {
	var a, exp, r gej_t
	var k num_t

	k.set_hex("84e5f7d329c3dab1160dbf9cb0b1a3c82e6058c06260f4101b1660b865ce98c5")
	a.x.set_hex("f46a67e20804f956a1ce64566d96a42658a9a7a4c9a0be924615bef881a4a3f2")
	a.y.set_hex("3a8218cdf4156c60585f5721189289cc89500eab79480a109eb1d0684e560996")
	a.z.set_hex("01")

	exp.x.set_hex("7ca947676876381329cefdf6bb58b409d56438a4be0786b4c899ea43b1c99e4d")
	exp.y.set_hex("eb75fbe5b68e0ed0e36e959099dc9b992123cb7c58f3ee22b6894b35966bd1ad")
	exp.z.set_hex("4372423267b6452929646ab1307cb1756412d5dd2c962a2e12c51f658e4ed0b8")

	a.twice(&r)
	if !r.equal(&exp) {
		t.Error("Twice() fail")
	}
}


func TestGejFpAdd(t *testing.T) {
	var a, ad, exp, r gej_t

	a.x.set_hex("1064e1c5c1f77227c497fb8b45710321642de0d725b4683e0ab6c8dbb79fa474")
	a.y.set_hex("66dead94e701f140abf1fa87781fe26c5c1c5b30b8ec25e25a246d113c40ee45")
	a.z.set_hex("a48793ae3fd901a773b22ca8a642763d28717369df6ec97cfc0ea94d6dc75375")

	ad.x.set_hex("f46a67e20804f956a1ce64566d96a42658a9a7a4c9a0be924615bef881a4a3f2")
	ad.y.set_hex("3a8218cdf4156c60585f5721189289cc89500eab79480a109eb1d0684e560996")
	ad.z.set_hex("01")

	exp.x.set_hex("adc99888418c4ddc0aacc18650c98407b0fa02fe726fd0e07a81049a73a8cc7a")
	exp.y.set_hex("978815885cd7382b06345dd9c3fefeaa2fa24b2e78b72ad43633a513dec6b5eb")
	exp.z.set_hex("624637786832d2583e1e27ab53d06fdc749293db0097438ff7ed3c46f19f9ac6")

	a.fp_add(&r, &ad)
	if !r.equal(&exp) {
		t.Error("FpAdd() fail 1")
	}

	a.x.set_hex("344325caaa8fcd06081c8b539b0daaf795a2e1de09f4ac915b55f6dfcc0f67f4")
	a.y.set_hex("1dcf49d655fd194150b1d5c3b606e04091a7b483acee8f696c8c5ac86af70c24")
	a.z.set_hex("32576efb35992c0794ab96913a4c0c7970e806087f9b2bb49d8ddcfa0cc61bfa")

	ad.x.set_hex("f46a67e20804f956a1ce64566d96a42658a9a7a4c9a0be924615bef881a4a3f2")
	ad.y.set_hex("c57de7320bea939fa7a0a8dee76d763376aff15486b7f5ef614e2f96b1a9f299")
	ad.z.set_hex("01")

	exp.x.set_hex("c3ef390a6079d8ab2ce3a44f0eb3ad7412271af3ae892725a58ba6ac76b3655e")
	exp.y.set_hex("23b3f3c6a210dcf5c92340787c0ce16b9ec4893ed3be075f3e7e1f63e85d93e2")
	exp.z.set_hex("366a5c7efd615197b8508d520d3f859d340e782c01ec917f675dda38cb8093c1")

	a.fp_add(&r, &ad)
	if !r.equal(&exp) {
		t.Error("FpAdd() fail 1")
	}
}


func TestGejFpMult(t *testing.T) {
	var a, exp, r gej_t
	var k num_t

	var vecs = [][6]string { // x, y, k (z always 1) -> x,y,x
		{
			"79be667ef9dcbbac55a06295ce870b07029bfcdb2dce28d959f2815b16f81798",
			"483ada7726a3c4655da4fbfc0e1108a8fd17b448a68554199c47d08ffb10d4b8",
			"ac5e6a9ad86a9f31a37201887d9c9b0f3e183d230ffcf2e31137cb00acc1c105",
			"3628a0d7d8621b1d6c60293b3aaea5fcebc6360f4e3252094267dae1ec831a60",
			"e0c2e6fa2b164a0aa7ce31f37e7cc1d2d4431a13cef69559f0f62931066fdbff",
			"cc0764cb49940d20edda6b87178c102e1a949a44ea8524110657dce311e6270d",
		},
		{
			"f46a67e20804f956a1ce64566d96a42658a9a7a4c9a0be924615bef881a4a3f2",
			"3a8218cdf4156c60585f5721189289cc89500eab79480a109eb1d0684e560996",
			"84e5f7d329c3dab1160dbf9cb0b1a3c82e6058c06260f4101b1660b865ce98c5",
			"eb8d6a5c12e70b0d5e05336e9103318e89ca4445004afc3640d3e47e488a4d0f",
			"8fabd090eade40104431906d3bc0c25d988270aa017bfa8ce3707c0d72649571",
			"631bcaec3954477e0dd17b3fa60d395a23ea7a47a7ad602c6372a6b6efb41ee3",
		},
		{
			"79be667ef9dcbbac55a06295ce870b07029bfcdb2dce28d959f2815b16f81798",
			"483ada7726a3c4655da4fbfc0e1108a8fd17b448a68554199c47d08ffb10d4b8",
			"a8d65dd8aaa1ab31c91fb18c8a18ac7a07bf649ec5ff15ef9255ad682912cada",
			"cda5a5dff4459558c8edc20e57bc5babc2a39dcba125fc75196bb5e0e04b4b54",
			"5d7612473f88d6cd522b09903ae08912aec39c4d7a508fa442382c6d57d58d24",
			"eaed65fd764f7d3d06068906b3402463785da475b995e284a48b4365b1c99151",
		},
		{
			"79be667ef9dcbbac55a06295ce870b07029bfcdb2dce28d959f2815b16f81798",
			"483ada7726a3c4655da4fbfc0e1108a8fd17b448a68554199c47d08ffb10d4b8",
			"a23b57ac32e4f90e20de6d011fbb4c628016b3436fdb860917e7e019d0bc7126",
			"06760669f905712bb497e561d57104f44eabce9716738dee764507913eda5ce8",
			"4f8bac8b946f46a9580614a9770987cae19eb9b0fac0590056e4f4d8e923ee5b",
			"db76a101c645e72e1367cd1d4bed7c205c1674735199ec253d13223959aa9f8c",
		},
	}

	for i := range vecs {
		a.x.set_hex(vecs[i][0])
		a.y.set_hex(vecs[i][1])
		a.z.set_hex("01")
		k.set_hex(vecs[i][2])
		exp.x.set_hex(vecs[i][3])
		exp.y.set_hex(vecs[i][4])
		exp.z.set_hex(vecs[i][5])
		a.fp_mul(&r, &k)
		if !r.equal(&exp) {
			r.print("got")
			exp.print("exp")
			t.Fatal("FpMult() fail at", i)
		}
	}
}

