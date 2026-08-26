package script

import (
	"bytes"
	"testing"

	"github.com/piotrnar/gocoin/lib/btc"
)

// TestPushDataScript checks that pushDataScript() reproduces the exact
// same push-opcode encoding that Bitcoin Core's "CScript() << data" produces
// (see CScript::AppendDataSize in src/script/script.h). This is what
// FindAndDelete (delSig here) uses to locate a signature push inside
// scriptCode, so any mismatch with Core's encoding is consensus-relevant.
func TestPushDataScript(t *testing.T) {
	cases := []struct {
		length int
		want   []byte // expected header bytes (excluding the payload)
	}{
		{0, []byte{0x00}},
		{1, []byte{0x01}},
		{75, []byte{0x4b}},              // last length using a direct push opcode
		{76, []byte{0x4c, 0x4c}},        // OP_PUSHDATA1 boundary (was broken: used to emit just {0x4c})
		{77, []byte{0x4c, 0x4d}},        // OP_PUSHDATA1 <len>
		{252, []byte{0x4c, 0xfc}},       // OP_PUSHDATA1 <len> (was broken: CompactSize would switch encoding here)
		{253, []byte{0x4c, 0xfd}},       // OP_PUSHDATA1 <len> (was broken: CompactSize prefixes 0xfd here)
		{255, []byte{0x4c, 0xff}},       // OP_PUSHDATA1 <len>
		{256, []byte{0x4d, 0x00, 0x01}}, // OP_PUSHDATA2 <len_le16>
	}

	for _, c := range cases {
		data := make([]byte, c.length)
		for i := range data {
			data[i] = 0xAB
		}
		got := pushDataScript(data)
		if len(got) != len(c.want)+c.length {
			t.Fatalf("length %d: got total len %d, want %d", c.length, len(got), len(c.want)+c.length)
		}
		if !bytes.Equal(got[:len(c.want)], c.want) {
			t.Fatalf("length %d: got header % x, want % x", c.length, got[:len(c.want)], c.want)
		}
		if !bytes.Equal(got[len(c.want):], data) {
			t.Fatalf("length %d: payload mismatch", c.length)
		}
	}
}

// TestDelSigLongSignature is a regression test for a bug where delSig() used
// btc.PutVlen (a CompactSize/VarInt encoder) instead of a script push-opcode
// encoder to rebuild the "CScript() << vchSig" template it searches for.
// The two encodings only coincide for lengths below 0x4c (76), so a stack
// item of 76+ bytes used as the "signature" argument to OP_CHECKSIG /
// OP_CHECKMULTISIG would never be matched/removed from scriptCode, unlike in
// Bitcoin Core's FindAndDelete.
func TestDelSigLongSignature(t *testing.T) {
	// A 76-byte "signature" (real ECDSA sigs never get this big, but nothing
	// stops a script from putting an arbitrary blob in that stack position).
	sig := bytes.Repeat([]byte{0x7a}, 76)

	// Build a scriptCode that contains exactly one push of that blob,
	// correctly encoded the way Bitcoin Core would encode it (OP_PUSHDATA1
	// followed by the length byte and the data).
	where := append([]byte{btc.OP_PUSHDATA1, byte(len(sig))}, sig...)

	res, cnt := delSig(where, sig)
	if cnt != 1 {
		t.Fatalf("expected to find and remove 1 occurrence, found %d", cnt)
	}
	if len(res) != 0 {
		t.Fatalf("expected scriptCode to be empty after removal, got % x", res)
	}
}
