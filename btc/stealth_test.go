package btc

import (
	"bytes"
	"testing"
	"encoding/hex"
)


var _stealth_vecs = [][7]string { // x, y, k ->  DH
	{
		"d5b3853bbee336b551ff999b0b1d656e65a7649037ae0dcb02b3c4ff5f29e5be",
		"b389c8d94e6e8326b82367aee08d93f06bb4b344a40a2fe7024ffdcbab699f0f",
		"fa63521e333e4b9f6a98a142680d3aef4d8e7f79723ce0043691db55c36bd905",
		"e8fa3814fa662a8ade1cce233333c1eaa29046b6cdc46e44ad4f7e8bd8e6f98e", /*DH*/
	},
}


func TestStealthDH(t *testing.T) {
	for i := range _stealth_vecs {
		x, _ := hex.DecodeString(_stealth_vecs[i][0])
		y, _ := hex.DecodeString(_stealth_vecs[i][1])
		k, _ := hex.DecodeString(_stealth_vecs[i][2])
		exp, _ := hex.DecodeString(_stealth_vecs[i][3])

		res := StealthDH(append([]byte{0x04}, append(x, y...)...), k)
		if !bytes.Equal(exp, res) {
			println(hex.EncodeToString(exp))
			println(hex.EncodeToString(res))
			t.Error("StealthDiff() fail at", i)
		}
	}
}



func TestStealthAddr(t *testing.T) {
	var adrs = []string {
		"waPX7opcFsJo5B9iJjiXjsmBY2oETSyCCBuDXBjxGXrH4pQCiyNXXRiN73VHBx9otWzxsEcErY5eGaKmUKqJHT7rBXpVE34zB1gV8h",
		"vJmyoyfHgvkW2fRbqpANQircWiWDFMHtzyUxbcGsnUCX6z1jEjfArypDBNMeQdmsczkLVoSwYRZ5pS8YAxxQY7Q2m8SUXB2sZWjB6q",
	}

	for i := range adrs {
		a, e := NewStealthAddrFromString(adrs[i])
		if e != nil || a==nil {
			t.Error(i, e.Error())
		}
		s := a.String()
		if s != adrs[i] {
			t.Error(i, "Re-encode mismatch")
		}
	}
	a, e := NewStealthAddrFromString("1"+adrs[0])
	if e==nil || a!=nil {
			t.Error("Error expected")
	}
}


func BenchmarkStealthDH(b *testing.B) {
	xy, _ := hex.DecodeString("04d5b3853bbee336b551ff999b0b1d656e65a7649037ae0dcb02b3c4ff5f29e5beb389c8d94e6e8326b82367aee08d93f06bb4b344a40a2fe7024ffdcbab699f0f")
	e, _ := hex.DecodeString("fa63521e333e4b9f6a98a142680d3aef4d8e7f79723ce0043691db55c36bd905")
	b.ResetTimer()
	for j := 0; j < b.N; j++ {
		StealthDH(xy, e)
	}
}
