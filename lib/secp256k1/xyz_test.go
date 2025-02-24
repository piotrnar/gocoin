package secp256k1

import (
	"testing"
)

func TestGejDouble(t *testing.T) {
	var a, a_exp, r XYZ
	a.X.SetHex("79be667ef9dcbbac55a06295ce870b07029bfcdb2dce28d959f2815b16f81798")
	a.Y.SetHex("483ada7726a3c4655da4fbfc0e1108a8fd17b448a68554199c47d08ffb10d4b8")
	a.Z.SetHex("01")
	a_exp.X.SetHex("7D152C041EA8E1DC2191843D1FA9DB55B68F88FEF695E2C791D40444B365AFC2")
	a_exp.Y.SetHex("56915849F52CC8F76F5FD7E4BF60DB4A43BF633E1B1383F85FE89164BFADCBDB")
	a_exp.Z.SetHex("9075B4EE4D4788CABB49F7F81C221151FA2F68914D0AA833388FA11FF621A970")

	a.Double(&r)
	if !r.Equals(&a_exp) {
		t.Error("gej.Double failed")
	}
}

func TestGejMulLambda(t *testing.T) {
	var a, a_exp XYZ
	a.X.SetHex("0eaebcd1df2df853d66ce0e1b0fda07f67d1cabefde98514aad795b86a6ea66d")
	a.Y.SetHex("beb26b67d7a00e2447baeccc8a4cef7cd3cad67376ac1c5785aeebb4f6441c16")
	a.Z.SetHex("01")
	a_exp.X.SetHex("a45720c272cfa1f77f64be8a404a7d3149bd5410f9a173353f6eb75a5085ba98")
	a_exp.Y.SetHex("beb26b67d7a00e2447baeccc8a4cef7cd3cad67376ac1c5785aeebb4f6441c16")
	a_exp.Z.SetHex("01")
	a_lam := a
	a_lam.mul_lambda(&a_lam)
	if !a_lam.Equals(&a_exp) {
		t.Error("mul_lambda failed")
	}
}

func TestGejGetX(t *testing.T) {
	var a XYZ
	var X, exp Field
	a.X.SetHex("EB6752420B6BDB40A760AC26ADD7E7BBD080BF1DF6C0B009A0D310E4511BDF49")
	a.Y.SetHex("8E8CEB84E1502FC536FFE67967BC44314270A0B38C79865FFED5A85D138DCA6B")
	a.Z.SetHex("813925AF112AAB8243F8CCBADE4CC7F63DF387263028DE6E679232A73A7F3C31")

	exp.SetHex("fe00e013c244062847045ae7eb73b03fca583e9aa5dbd030a8fd1c6dfcf11b10")
	a.get_x(&X)
	if !X.Equals(&exp) {
		t.Error("get.get_x() fail")
	}
}
