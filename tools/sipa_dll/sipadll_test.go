package spiadll

import (
	"testing"
	"encoding/hex"
)

func TestVerify(t *testing.T) {
	pkey, _ := hex.DecodeString("040eaebcd1df2df853d66ce0e1b0fda07f67d1cabefde98514aad795b86a6ea66dbeb26b67d7a00e2447baeccc8a4cef7cd3cad67376ac1c5785aeebb4f6441c16")
	hasz, _ := hex.DecodeString("3382219555ddbb5b00e0090f469e590ba1eae03c7f28ab937de330aa60294ed6")
	sign, _ := hex.DecodeString("3045022100fe00e013c244062847045ae7eb73b03fca583e9aa5dbd030a8fd1c6dfcf11b1002207d0d04fed8fa1e93007468d5a9e134b0a7023b6d31db4e50942d43a250f4d07c01")
	res := EC_Verify(pkey, sign, hasz)
	if res!=1 {
		t.Error("Verify failed")
	}
	hasz[0]++
	res = EC_Verify(pkey, sign, hasz)
	if res!=0 {
		t.Error("Verify not failed while it should")
	}
	res = EC_Verify(pkey[:1], sign, hasz)
	if res>=0 {
		t.Error("Negative result expected", res)
	}
	res = EC_Verify(pkey, sign[:1], hasz)
	if res>=0 {
		t.Error("Yet negative result expected", res)
	}
	res = EC_Verify(pkey, sign, hasz[:1])
	if res!=0 {
		t.Error("Zero expected", res)
	}
}

func TestSchnorr(t *testing.T) {
	key, _ := hex.DecodeString("DFF1D77F2A671C5F36183726DB2341BE58FEAE1DA2DECED843240F7B502BA659")
	sig, _ := hex.DecodeString("6896BD60EEAE296DB48A229FF71DFE071BDE413E6D43F917DC8DCF8C78DE33418906D11AC976ABCCB20B091292BFF4EA897EFCB639EA871CFA95F6DE339E4B0A")
	msg, _ := hex.DecodeString("243F6A8885A308D313198A2E03707344A4093822299F31D0082EFA98EC4E6C89")
	res := Schnorr_Verify(key, sig, msg)
	if res!=1 {
		t.Error("Schnorr Verify failed")
	}
	msg[0]++
	res = Schnorr_Verify(key, sig, msg)
	if res!=0 {
		t.Error("Schnorr Verify not failed while it should")
	}
}

func TestPay2Scr(t *testing.T) {
	pkey, _ := hex.DecodeString("afaf8a67be00186668f74740e34ffce748139c2b73c9fbd2c1f33e48a612a75d")
	base, _ := hex.DecodeString("f1cbd3f2430910916144d5d2bf63d48a6281e5b8e6ade31413adccff3d8839d4")
	hash, _ := hex.DecodeString("93a760e87123883022cbd462ac40571176cf09d9d2c6168759fee6c2b079fdd8")
	var parity int = 1
	res := CheckPayToContract(pkey, base, hash, parity)
	if res!=1 {
		t.Error("CheckPayToContract failed")
	}
	hash[0]++
	res = CheckPayToContract(pkey, base, hash, parity)
	if res!=0 {
		t.Error("CheckPayToContract not failed while it should")
	}
}
