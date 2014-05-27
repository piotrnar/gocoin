package btc

import (
	"fmt"
	"bytes"
	"testing"
	"encoding/hex"
	"encoding/binary"
)


var _stealth_vecs = [][7]string { // x, y, k ->  DH
	{
		"d5b3853bbee336b551ff999b0b1d656e65a7649037ae0dcb02b3c4ff5f29e5be",
		"b389c8d94e6e8326b82367aee08d93f06bb4b344a40a2fe7024ffdcbab699f0f",
		"fa63521e333e4b9f6a98a142680d3aef4d8e7f79723ce0043691db55c36bd905",
		//"2a7a04b8d095a27e2161b623fbd11dca0a35e56f66d3794421f007a62857cfb6", /*DH broken one*/
		"8849a7e0761c81b6398894e915a9fd62d77a07d10a60a4b1604b9087f7a1aa49", /*DH fixed one*/
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


func TestCheckPrefix(t *testing.T) {
	var px [5]byte
	var pat [4]byte
	sa := new(StealthAddr)
	mask := uint32(0xffffffff)
	for i:=byte(0); i<32; i++ {
		px[0] = i
		sa.Prefix = px[:1+(byte(i+7)>>3)]

		val := uint32(0xdeadbeef)
		binary.BigEndian.PutUint32(px[1:], val)

		nval := val^mask
		binary.BigEndian.PutUint32(pat[:], nval)
		if !sa.CheckPrefix(pat[:]) {
			t.Fatal(fmt.Sprintf("CheckPrefix failed. i=%d  val=0x%08x  pat=0x%08x  msk=0x%08x", i, val, nval, mask))
		}

		nval ^= uint32(uint64(0x100000000)>>i)
		binary.BigEndian.PutUint32(pat[:], nval)
		if sa.CheckPrefix(pat[:]) {
			if i!=0 {
				t.Fatal(fmt.Sprintf("CheckPrefix not failed. i=%d  val=0x%08x  pat=0x%08x  msk=0x%08x", i, val, nval, mask))
			}
		} else if i==0 {
			t.Fatal(fmt.Sprintf("CheckPrefix failed. i=%d  val=0x%08x  pat=0x%08x  msk=0x%08x", i, val, nval, mask))
		}

		mask >>= 1
	}
}
