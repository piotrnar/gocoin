package btc

import (
	"encoding/binary"
	"encoding/hex"
	"testing"
)

// A known-good, minimal non-segwit transaction (1 input, 2 outputs), reused
// from the ecdsa test vectors. Used to prove the guards don't reject valid txs.
const validRawTxHex = "01000000014d276db8e3a547cc3eaff4051d0d158da21724634d7c67c51129fa403dded5de010000001976a914718950ac3039e53fbd6eb0213de333b689a1ca1288acffffffff02a8d39b0f000000001976a914db641fc6dff262fe2504725f2c4c1852b18ffe3588ace693f205000000001976a9141321c4f37c5b2be510c1c7725a83e561ad27876b88ac00000000"

// compactSize encodes n as a Bitcoin CompactSize / var_int.
func compactSize(n uint64) []byte {
	switch {
	case n < 0xfd:
		return []byte{byte(n)}
	case n <= 0xffff:
		b := []byte{0xfd, 0, 0}
		binary.LittleEndian.PutUint16(b[1:], uint16(n))
		return b
	case n <= 0xffffffff:
		b := []byte{0xfe, 0, 0, 0, 0}
		binary.LittleEndian.PutUint32(b[1:], uint32(n))
		return b
	default:
		b := []byte{0xff, 0, 0, 0, 0, 0, 0, 0, 0}
		binary.LittleEndian.PutUint64(b[1:], n)
		return b
	}
}

// Attacker-controlled counts that would allocate enormous slices if unchecked.
// 1<<31 alone is ~16 GB of pointers; the last one overflows via the +1 paths.
var evilCounts = []uint64{1 << 31, 1 << 40, 1 << 52, 1<<64 - 1}

// TestNewTxRejectsHugeInputCount feeds a tiny payload whose TxIn CompactSize
// names billions of inputs. Before the fix this reached make([]*TxIn, le),
// causing an unrecoverable runtime OOM throw (not caught by NewTx's recover()).
// After the fix NewTx must return (nil, 0) instead.
func TestNewTxRejectsHugeInputCount(t *testing.T) {
	for _, cnt := range evilCounts {
		pl := []byte{0x01, 0x00, 0x00, 0x00} // version
		pl = append(pl, compactSize(cnt)...) // TxIn count
		tx, off := NewTx(pl)
		if tx != nil || off != 0 {
			t.Fatalf("input count %d: expected (nil,0), got tx!=nil=%v off=%d", cnt, tx != nil, off)
		}
	}
}

// TestNewTxRejectsHugeOutputCount does the same for the TxOut count. The payload
// carries one real, fully-formed input so parsing reaches the TxOut count field.
func TestNewTxRejectsHugeOutputCount(t *testing.T) {
	for _, cnt := range evilCounts {
		pl := []byte{0x01, 0x00, 0x00, 0x00} // version
		pl = append(pl, 0x01)                // 1 input
		pl = append(pl, make([]byte, 32)...) // prevout hash
		pl = append(pl, 0, 0, 0, 0)          // prevout index
		pl = append(pl, 0x00)                // empty scriptSig
		pl = append(pl, 0xff, 0xff, 0xff, 0xff)
		pl = append(pl, compactSize(cnt)...) // TxOut count
		tx, off := NewTx(pl)
		if tx != nil || off != 0 {
			t.Fatalf("output count %d: expected (nil,0), got tx!=nil=%v off=%d", cnt, tx != nil, off)
		}
	}
}

// TestNewTxRejectsHugeScriptLen targets the per-output Pk_script length, which
// drives make([]byte, le) in NewTxOut.
func TestNewTxRejectsHugeScriptLen(t *testing.T) {
	for _, cnt := range evilCounts {
		pl := []byte{0x01, 0x00, 0x00, 0x00}
		pl = append(pl, 0x01)
		pl = append(pl, make([]byte, 32)...)
		pl = append(pl, 0, 0, 0, 0)
		pl = append(pl, 0x00)
		pl = append(pl, 0xff, 0xff, 0xff, 0xff)
		pl = append(pl, 0x01)                // 1 output
		pl = append(pl, make([]byte, 8)...)  // value
		pl = append(pl, compactSize(cnt)...) // pk_script length
		tx, off := NewTx(pl)
		if tx != nil || off != 0 {
			t.Fatalf("script len %d: expected (nil,0), got tx!=nil=%v off=%d", cnt, tx != nil, off)
		}
	}
}

// TestNewTxParsesValidTx guards against an over-aggressive check: a legitimate
// transaction must still parse and consume exactly its whole buffer.
func TestNewTxParsesValidTx(t *testing.T) {
	raw, err := hex.DecodeString(validRawTxHex)
	if err != nil {
		t.Fatal(err)
	}
	tx, off := NewTx(raw)
	if tx == nil {
		t.Fatal("valid tx was rejected")
	}
	if off != len(raw) {
		t.Fatalf("offset = %d, want %d", off, len(raw))
	}
	if len(tx.TxIn) != 1 || len(tx.TxOut) != 2 {
		t.Fatalf("parsed %d ins / %d outs, want 1 / 2", len(tx.TxIn), len(tx.TxOut))
	}
}

// TestBuildTxListParsesValidBlock guards the block path against an
// over-aggressive check: a minimal well-formed block (header + 1 tx) must still
// build its tx list. Built by hand so the test needs no network access.
func TestBuildTxListParsesValidBlock(t *testing.T) {
	rawtx, err := hex.DecodeString(validRawTxHex)
	if err != nil {
		t.Fatal(err)
	}
	raw := make([]byte, 80) // header (only length matters for BuildTxList)
	raw = append(raw, 0x01) // txn_count = 1
	raw = append(raw, rawtx...)

	bl := &Block{Raw: raw}
	if e := bl.BuildTxListExt(false); e != nil {
		t.Fatalf("valid block was rejected: %v", e)
	}
	if len(bl.Txs) != 1 || bl.Txs[0] == nil {
		t.Fatalf("expected 1 parsed tx, got %d", len(bl.Txs))
	}
}

// TestBuildTxListRejectsHugeTxCount covers the block path. bl.Txs is allocated
// from bl.TxCount *before* any NewTx call, so the NewTx guard does not protect
// it; BuildTxListExt needs its own bound. An 89-byte block naming billions of
// txs must return an error instead of OOM-crashing.
func TestBuildTxListRejectsHugeTxCount(t *testing.T) {
	for _, cnt := range evilCounts {
		raw := make([]byte, 80)              // header
		raw = append(raw, compactSize(cnt)...) // txn_count
		bl := &Block{Raw: raw}
		e := bl.BuildTxListExt(false)
		if e == nil {
			t.Fatalf("txn_count %d: expected an error, got nil", cnt)
		}
	}
}
