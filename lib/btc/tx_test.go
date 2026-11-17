package btc

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

func testTx(segwit bool) *Tx {
	tx := &Tx{Version: 2, Lock_time: 0x11223344}
	tx.TxIn = []*TxIn{{ScriptSig: []byte{1, 2, 3}, Sequence: 5}, {Sequence: 0xffffffff}}
	tx.TxIn[0].Input.Hash[0] = 0xaa
	tx.TxIn[1].Input.Hash[31] = 0xbb
	tx.TxIn[1].Input.Vout = 7
	tx.TxOut = []*TxOut{{Value: 7, Pk_script: []byte{9, 9}}, {Value: 0x0102030405060708}}
	if segwit {
		tx.SegWit = [][][]byte{{{1, 2, 3, 4}, {}, {5}}, {}}
	}
	return tx
}

func TestTxSerialize(t *testing.T) {
	const legacy = "02000000" + "02" +
		"aa00000000000000000000000000000000000000000000000000000000000000" + "00000000" + "03010203" + "05000000" +
		"00000000000000000000000000000000000000000000000000000000000000bb" + "07000000" + "00" + "ffffffff" +
		"02" + "0700000000000000" + "020909" + "0807060504030201" + "00" +
		"44332211"
	const witness = "02000000" + "0001" + "02" +
		"aa00000000000000000000000000000000000000000000000000000000000000" + "00000000" + "03010203" + "05000000" +
		"00000000000000000000000000000000000000000000000000000000000000bb" + "07000000" + "00" + "ffffffff" +
		"02" + "0700000000000000" + "020909" + "0807060504030201" + "00" +
		"03" + "0401020304" + "00" + "0105" + "00" +
		"44332211"

	tx := testTx(false)
	if s := hex.EncodeToString(tx.Serialize()); s != legacy {
		t.Fatal("Serialize:", s)
	}
	if s := hex.EncodeToString(tx.SerializeNew()); s != legacy {
		t.Fatal("SerializeNew of non-segwit tx:", s)
	}

	tx = testTx(true)
	if s := hex.EncodeToString(tx.Serialize()); s != legacy {
		t.Fatal("Serialize of segwit tx:", s)
	}
	if s := hex.EncodeToString(tx.SerializeNew()); s != witness {
		t.Fatal("SerializeNew:", s)
	}
	wr := new(bytes.Buffer)
	tx.WriteSerialized(wr)
	if hex.EncodeToString(wr.Bytes()) != legacy {
		t.Fatal("WriteSerialized")
	}
	wr.Reset()
	tx.WriteSerializedNew(wr)
	if hex.EncodeToString(wr.Bytes()) != witness {
		t.Fatal("WriteSerializedNew")
	}

	// and back
	raw, _ := hex.DecodeString(witness)
	tx2, n := NewTx(raw)
	if tx2 == nil || n != len(raw) {
		t.Fatal("NewTx failed")
	}
	tx2.SetHash(raw)
	if int(tx2.NoWitSize) != len(legacy)/2 || int(tx2.Size) != len(raw) {
		t.Fatal("wrong sizes:", tx2.NoWitSize, tx2.Size)
	}
	if !bytes.Equal(tx2.SerializeNew(), raw) {
		t.Fatal("round trip failed")
	}
	leg, _ := hex.DecodeString(legacy)
	if tx2.Hash.Hash != Sha2Sum(leg) || tx2.WTxID().Hash != Sha2Sum(raw) {
		t.Fatal("wrong hashes")
	}
}

func TestTxSize(t *testing.T) {
	for _, segwit := range []bool{false, true} {
		raw := testTx(segwit).SerializeNew()
		if n := TxSize(raw); n != len(raw) {
			t.Fatal("wrong size:", n, len(raw))
		}
		if n := TxSize(append(raw, 0xaa, 0xbb, 0xcc)); n != len(raw) {
			t.Fatal("wrong size with extra data:", n, len(raw))
		}
		// a truncated tx must never be reported as a valid one
		for i := 0; i < len(raw); i++ {
			if n := TxSize(raw[:i]); n != 0 {
				t.Fatal("tx truncated at", i, "returned size", n)
			}
		}
	}

	// Huge inputs count, with the data ending just after the first input's outpoint.
	// It used to spin for (almost) ever, not advancing in the buffer.
	b := []byte{1, 0, 0, 0, 0xff, 0, 0, 0, 0, 0, 1, 0, 0}
	b = append(b, make([]byte, 36)...)
	if n := TxSize(b); n != 0 {
		t.Fatal("corrupt tx returned size", n)
	}
	// ... same for outputs
	b = []byte{1, 0, 0, 0, 0 /*no inputs*/, 0xff, 0, 0, 0, 0, 0, 1, 0, 0}
	b = append(b, make([]byte, 8)...)
	if n := TxSize(b); n != 0 {
		t.Fatal("corrupt tx returned size", n)
	}
}

func TestNewTxSuperfluousWitness(t *testing.T) {
	tx := testTx(true)
	tx.SegWit = [][][]byte{{}, {}}
	if x, _ := NewTx(tx.SerializeNew()); x != nil {
		t.Fatal("segwit encoding with all the witness stacks empty must be rejected")
	}
	tx.SegWit = [][][]byte{{}, {{}}} // an empty item still makes it a witness
	raw := tx.SerializeNew()
	if x, n := NewTx(raw); x == nil || n != len(raw) || len(x.SegWit[1]) != 1 {
		t.Fatal("valid segwit tx rejected")
	}
}

func TestSigOpCountAfterOpReturn(t *testing.T) {
	// OP_RETURN OP_CHECKSIG OP_CHECKMULTISIG <push of two bytes that look like OP_CHECKSIG>
	scr := []byte{OP_RETURN, 0xac, 0xae, 0x02, 0xac, 0xac}
	if n := GetSigOpCount(scr, false); n != 1+MAX_PUBKEYS_PER_MULTISIG {
		t.Fatal("wrong sigops count:", n)
	}
	// OP_3 OP_CHECKMULTISIG, then a truncated push
	scr = []byte{OP_3, 0xae, 0x05, 0xac}
	if n := GetSigOpCount(scr, true); n != 3 {
		t.Fatal("wrong accurate sigops count:", n)
	}
	if n := GetSigOpCount(scr, false); n != MAX_PUBKEYS_PER_MULTISIG {
		t.Fatal("wrong legacy sigops count:", n)
	}
}

func TestWritePutLen(t *testing.T) {
	for _, le := range []uint32{0, 1, 75, 76, 77, 255, 256, 65535, 65536} {
		wr := new(bytes.Buffer)
		WritePutLen(wr, le)
		wr.Write(make([]byte, le))
		opcode, data, n, er := GetOpcode(wr.Bytes())
		if er != nil || n != wr.Len() || len(data) != int(le) || opcode > OP_PUSHDATA4 {
			t.Fatal("push of", le, "bytes does not decode:", er, n, len(data))
		}
		if !bytes.Equal(wr.Bytes(), RawToStack(make([]byte, le))) && le > 1 {
			t.Fatal("push of", le, "bytes differs from RawToStack")
		}
	}
}

func TestVlen(t *testing.T) {
	for _, c := range []struct {
		val uint64
		enc string
	}{{0, "00"}, {0xfc, "fc"}, {0xfd, "fdfd00"}, {0xffff, "fdffff"}, {0x10000, "fe00000100"},
		{0xffffffff, "feffffffff"}, {0x100000000, "ff0000000001000000"}, {0x7fffffffffffffff, "ffffffffffffffff7f"}} {
		enc := AppendVlen([]byte{0x55}, c.val)
		if hex.EncodeToString(enc[1:]) != c.enc || enc[0] != 0x55 {
			t.Fatal("AppendVlen:", c.val, hex.EncodeToString(enc))
		}
		wr := new(bytes.Buffer)
		WriteVlen(wr, c.val)
		if hex.EncodeToString(wr.Bytes()) != c.enc || VLenSize(c.val) != wr.Len() {
			t.Fatal("WriteVlen:", c.val)
		}
		if v, n := VULe(wr.Bytes()); v != c.val || n != wr.Len() {
			t.Fatal("VULe:", c.val)
		}
		if v, er := ReadVLen(wr); er != nil || v != c.val {
			t.Fatal("ReadVLen:", c.val)
		}
	}
}

func TestBlockTrailingData(t *testing.T) {
	raw := testTx(true).SerializeNew()
	for _, dohash := range []bool{false, true} {
		blk := append(make([]byte, 80), 2) // two transactions
		blk = append(blk, raw...)
		blk = append(blk, testTx(false).Serialize()...)
		exact := len(blk)

		bl, _ := NewBlock(blk)
		if er := bl.BuildTxListExt(dohash); er != nil || len(bl.Raw) != exact || len(bl.Txs) != 2 {
			t.Fatal("block with no extra data:", er, len(bl.Raw))
		}
		weight := bl.BlockWeight

		// the data that follow the last tx shall be ignored and cut off
		bl, _ = NewBlock(append(blk, 1, 2, 3, 4, 5))
		if er := bl.BuildTxListExt(dohash); er != nil || len(bl.Raw) != exact || len(bl.Txs) != 2 {
			t.Fatal("block with extra data:", er, len(bl.Raw))
		}
		if bl.BlockWeight != weight || !bytes.Equal(bl.Txs[0].Raw, raw) {
			t.Fatal("extra data changed the block's content")
		}
		if dohash && bl.Txs[0].Hash.Hash != Sha2Sum(testTx(false).Serialize()) {
			t.Fatal("wrong txid of a segwit tx")
		}
	}
}

func TestHMAC(t *testing.T) {
	for _, keylen := range []int{0, 1, 32, 63, 64, 65, 200} {
		key := bytes.Repeat([]byte{0x5a}, keylen)
		var out [32]byte
		h := HMAC_Init(key)
		h.Write([]byte("some message"))
		h.Finalize(out[:])
		ref := hmac.New(sha256.New, key)
		ref.Write([]byte("some message"))
		if !bytes.Equal(ref.Sum(nil), out[:]) {
			t.Fatal("HMAC mismatch for key length", keylen)
		}
	}
}

func TestCalcMerkle(t *testing.T) {
	pair := func(a, b [32]byte) [32]byte {
		return Sha2Sum(append(a[:], b[:]...))
	}
	var h [5][32]byte
	for i := range h {
		h[i][0] = byte(i + 1)
	}
	mk := func(n int) [][32]byte {
		return append(make([][32]byte, 0, 3*n), h[:n]...)
	}

	if res, mut := CalcMerkle(mk(1)); !bytes.Equal(res, h[0][:]) || mut {
		t.Fatal("one tx")
	}
	exp := pair(h[0], h[1])
	if res, mut := CalcMerkle(mk(2)); !bytes.Equal(res, exp[:]) || mut {
		t.Fatal("two txs")
	}
	exp = pair(pair(h[0], h[1]), pair(h[2], h[2]))
	if res, mut := CalcMerkle(mk(3)); !bytes.Equal(res, exp[:]) || mut {
		t.Fatal("three txs")
	}
	exp = pair(pair(pair(h[0], h[1]), pair(h[2], h[3])), pair(pair(h[4], h[4]), pair(h[4], h[4])))
	if res, mut := CalcMerkle(mk(5)); !bytes.Equal(res, exp[:]) || mut {
		t.Fatal("five txs")
	}

	// a duplicated tx gives the same root as the three txs, but must be reported as a mutation
	m := mk(3)
	m = append(m, h[2])
	exp = pair(pair(h[0], h[1]), pair(h[2], h[2]))
	if res, mut := CalcMerkle(m); !bytes.Equal(res, exp[:]) || !mut {
		t.Fatal("mutation not detected")
	}
}
