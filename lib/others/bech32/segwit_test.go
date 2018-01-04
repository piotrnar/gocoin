package bech32

import (
	"bytes"
	"strings"
	"testing"
)

type valid_address_data struct {
	address      string
	scriptPubKey []byte
}

type invalid_address_data struct {
	hrp            string
	version        int
	program_length int
}

var valid_address = []valid_address_data{
	{
		address: "BC1QW508D6QEJXTDG4Y5R3ZARVARY0C5XW7KV8F3T4",
		scriptPubKey: []byte{
			0x00, 0x14, 0x75, 0x1e, 0x76, 0xe8, 0x19, 0x91, 0x96, 0xd4, 0x54,
			0x94, 0x1c, 0x45, 0xd1, 0xb3, 0xa3, 0x23, 0xf1, 0x43, 0x3b, 0xd6}},
	{
		address: "tb1qrp33g0q5c5txsp9arysrx4k6zdkfs4nce4xj0gdcccefvpysxf3q0sl5k7",
		scriptPubKey: []byte{
			0x00, 0x20, 0x18, 0x63, 0x14, 0x3c, 0x14, 0xc5, 0x16, 0x68, 0x04,
			0xbd, 0x19, 0x20, 0x33, 0x56, 0xda, 0x13, 0x6c, 0x98, 0x56, 0x78,
			0xcd, 0x4d, 0x27, 0xa1, 0xb8, 0xc6, 0x32, 0x96, 0x04, 0x90, 0x32,
			0x62}},
	{
		address: "bc1pw508d6qejxtdg4y5r3zarvary0c5xw7kw508d6qejxtdg4y5r3zarvary0c5xw7k7grplx",
		scriptPubKey: []byte{
			0x51, 0x28, 0x75, 0x1e, 0x76, 0xe8, 0x19, 0x91, 0x96, 0xd4, 0x54,
			0x94, 0x1c, 0x45, 0xd1, 0xb3, 0xa3, 0x23, 0xf1, 0x43, 0x3b, 0xd6,
			0x75, 0x1e, 0x76, 0xe8, 0x19, 0x91, 0x96, 0xd4, 0x54, 0x94, 0x1c,
			0x45, 0xd1, 0xb3, 0xa3, 0x23, 0xf1, 0x43, 0x3b, 0xd6}},
	{
		address: "BC1SW50QA3JX3S",
		scriptPubKey: []byte{
			0x60, 0x02, 0x75, 0x1e}},
	{
		address: "bc1zw508d6qejxtdg4y5r3zarvaryvg6kdaj",
		scriptPubKey: []byte{
			0x52, 0x10, 0x75, 0x1e, 0x76, 0xe8, 0x19, 0x91, 0x96, 0xd4, 0x54,
			0x94, 0x1c, 0x45, 0xd1, 0xb3, 0xa3, 0x23}},
	{
		address: "tb1qqqqqp399et2xygdj5xreqhjjvcmzhxw4aywxecjdzew6hylgvsesrxh6hy",
		scriptPubKey: []byte{
			0x00, 0x20, 0x00, 0x00, 0x00, 0xc4, 0xa5, 0xca, 0xd4, 0x62, 0x21,
			0xb2, 0xa1, 0x87, 0x90, 0x5e, 0x52, 0x66, 0x36, 0x2b, 0x99, 0xd5,
			0xe9, 0x1c, 0x6c, 0xe2, 0x4d, 0x16, 0x5d, 0xab, 0x93, 0xe8, 0x64,
			0x33}}}

var invalid_address = []string{
	"tc1qw508d6qejxtdg4y5r3zarvary0c5xw7kg3g4ty",
	"bc1qw508d6qejxtdg4y5r3zarvary0c5xw7kv8f3t5",
	"BC13W508D6QEJXTDG4Y5R3ZARVARY0C5XW7KN40WF2",
	"bc1rw5uspcuh",
	"bc10w508d6qejxtdg4y5r3zarvary0c5xw7kw508d6qejxtdg4y5r3zarvary0c5xw7kw5rljs90",
	"BC1QR508D6QEJXTDG4Y5R3ZARVARYV98GJ9P",
	"tb1qrp33g0q5c5txsp9arysrx4k6zdkfs4nce4xj0gdcccefvpysxf3q0sL5k7",
	"bc1zw508d6qejxtdg4y5r3zarvaryvqyzf3du",
	"tb1qrp33g0q5c5txsp9arysrx4k6zdkfs4nce4xj0gdcccefvpysxf3pjxtptv",
	"bc1gmk9yu"}

var invalid_address_enc = []invalid_address_data{
	{hrp: "BC", version: 0, program_length: 20},
	{hrp: "bc", version: 0, program_length: 21},
	{hrp: "bc", version: 17, program_length: 32},
	{hrp: "bc", version: 1, program_length: 1},
	{hrp: "bc", version: 16, program_length: 41}}

func segwit_scriptpubkey(witver int, witprog []byte) (scriptpubkey []byte) {
	scriptpubkey = make([]byte, len(witprog)+2)
	if witver != 0 {
		scriptpubkey[0] = byte(0x50 + witver)
	}
	scriptpubkey[1] = byte(len(witprog))
	copy(scriptpubkey[2:], witprog)
	return
}

func TestValidAddress(t *testing.T) {
	for _, rec := range valid_address {
		hrp := "bc"
		witver, witprog := SegwitDecode(hrp, rec.address)
		if witprog == nil {
			hrp = "tb"
			witver, witprog = SegwitDecode(hrp, rec.address)
		}
		if witprog == nil {
			t.Error("SegwitDecode fails: ", rec.address)
			continue
		}
		scriptpubkey := segwit_scriptpubkey(witver, witprog)
		if !bytes.Equal(scriptpubkey, rec.scriptPubKey) {
			t.Error("SegwitDecode produces wrong result: ", rec.address)
			continue
		}
		rebuild := SegwitEncode(hrp, witver, witprog)
		if rebuild == "" {
			t.Error("SegwitEncode fails: ", rec.address)
			continue
		}
		if !strings.EqualFold(rec.address, rebuild) {
			t.Error("SegwitEncode produces wrong result: ", rec.address)
		}
	}
}

func TestInvalidAddress(t *testing.T) {
	for _, s := range invalid_address {
		_, witprog := SegwitDecode("bc", s)
		if witprog != nil {
			t.Error("SegwitDecode succeeds on invalid address: ", s)
		}
		_, witprog = SegwitDecode("tb", s)
		if witprog != nil {
			t.Error("SegwitDecode succeeds on invalid address: ", s)
		}
	}
}

func TestInvalidAddressEnc(t *testing.T) {
	for _, rec := range invalid_address_enc {
		rebuild := SegwitEncode(rec.hrp, rec.version, []byte{0})
		if rebuild != "" {
			t.Error("SegwitEncode succeeds on invalid input: ", rebuild)
		}
	}
}
