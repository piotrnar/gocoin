package openssl

import (
	"testing"
	"encoding/hex"
)

func TestVerify(t *testing.T) {
	pkey, _ := hex.DecodeString("040eaebcd1df2df853d66ce0e1b0fda07f67d1cabefde98514aad795b86a6ea66dbeb26b67d7a00e2447baeccc8a4cef7cd3cad67376ac1c5785aeebb4f6441c16")
	hasz, _ := hex.DecodeString("3382219555ddbb5b00e0090f469e590ba1eae03c7f28ab937de330aa60294ed6")
	sign, _ := hex.DecodeString("3045022100fe00e013c244062847045ae7eb73b03fca583e9aa5dbd030a8fd1c6dfcf11b1002207d0d04fed8fa1e93007468d5a9e134b0a7023b6d31db4e50942d43a250f4d07c")
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
	if res!=-2 {
		t.Error("Negative result expected", res)
	}
	res = EC_Verify(pkey, sign[:1], hasz)
	if res!=-1 {
		t.Error("Yet negative result expected", res)
	}
	res = EC_Verify(pkey, sign, hasz[:1])
	if res!=0 {
		t.Error("Zero expected", res)
	}
}
