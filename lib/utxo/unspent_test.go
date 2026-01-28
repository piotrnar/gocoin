package utxo

import (
	"bytes"
	"encoding/hex"
	"testing"

	"github.com/piotrnar/gocoin/lib/btc"
)

const (
	UtxoRecord = "B26B877AF9D16E5F634C4997A8393C9496BAA14C34D73829767723D96D4AE368FE19AC0700060100166A146F6D6E69000000000000001F0000008B3B93DC0002FD22021976A914A25DEC4D0011064EF106A983C39C7A540699F22088AC"
	//UtxoRecord = "875207AE844E25A60BB57C7E68FDEA8C3BD04FBF678866EF3E7E9FDD408B9E98FEF07A06000401FD60EA17A914379238E99325F2BD2D1F773B8D95CFB9EA92C31887"
)

func TestFullUtxoRec(t *testing.T) {
	raw, _ := hex.DecodeString(UtxoRecord)
	rec := FullUtxoRec(raw)
	if rec == nil {
		t.Error("nil returned")
		return
	}
	refid := btc.NewUint256FromString("68e34a6dd92377762938d7344ca1ba96943c39a897494c635f6ed1f97a876bb2")
	txid := btc.NewUint256(rec.TxID[:])
	if !refid.Equal(txid) {
		t.Error("TxID mismatch")
	}
	if rec.Coinbase {
		t.Error("Coibase mismatch")
	}
	if rec.InBlock != 502809 {
		t.Error("InBlock mismatch")
	}
	if len(rec.Outs) != 3 {
		t.Error("Outs count mismatch")
	}
	if rec.Outs[0] != nil {
		t.Error("Outs[0] not nil")
	}
	if rec.Outs[1] == nil {
		t.Error("Outs[1] is nil")
	}
	if rec.Outs[1].Value != 0 {
		t.Error("Outs[1] bad value")
	}
	if len(rec.Outs[1].PKScr) != 22 {
		t.Error("Outs[1] bad script")
	}
	if rec.Outs[2].Value != 546 {
		t.Error("Outs[2] bad value")
	}
	if len(rec.Outs[2].PKScr) != 25 {
		t.Error("Outs[2] bad script")
	}
	if !bytes.Equal(raw, Serialize(rec, true, nil)) {
		t.Error("Serialize error")
	}
}

func BenchmarkFullUtxoRec(b *testing.B) {
	raw, _ := hex.DecodeString(UtxoRecord)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if FullUtxoRec(raw) == nil {
			b.Fatal("Nil pointer returned")
		}
	}
}

func BenchmarkNewUtxoRec(b *testing.B) {
	raw, _ := hex.DecodeString(UtxoRecord)
	var key UtxoKeyType
	copy(key[:], raw[:])
	dat := raw[UtxoIdxLen:]
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if NewUtxoRec(key, dat) == nil {
			b.Fatal("Nil pointer returned")
		}
	}
}

func BenchmarkNewUtxoRecStatic(b *testing.B) {
	raw, _ := hex.DecodeString(UtxoRecord)
	var key UtxoKeyType
	copy(key[:], raw[:])
	dat := raw[UtxoIdxLen:]
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if NewUtxoRecStatic(key, dat) == nil {
			b.Fatal("Nil pointer returned")
		}
	}
}

func BenchmarkSerialize(b *testing.B) {
	raw, _ := hex.DecodeString(UtxoRecord)
	var key UtxoKeyType
	var buf [0x100000]byte
	copy(key[:], raw[:])
	dat := raw[UtxoIdxLen:]
	rec := NewUtxoRecStatic(key, dat)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if Serialize(rec, false, buf[:]) == nil {
			b.Fatal("Nil pointer returned")
		}
	}
}

func BenchmarkSerializeFull(b *testing.B) {
	raw, _ := hex.DecodeString(UtxoRecord)
	var key UtxoKeyType
	var buf [0x100000]byte
	copy(key[:], raw[:])
	dat := raw[UtxoIdxLen:]
	rec := NewUtxoRecStatic(key, dat)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if Serialize(rec, false, buf[:]) == nil {
			b.Fatal("Nil pointer returned")
		}
	}
}

func BenchmarkSerializeWithAlloc(b *testing.B) {
	raw, _ := hex.DecodeString(UtxoRecord)
	var key UtxoKeyType
	copy(key[:], raw[:])
	dat := raw[UtxoIdxLen:]
	rec := NewUtxoRecStatic(key, dat)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if Serialize(rec, false, nil) == nil {
			b.Fatal("Nil pointer returned")
		}
	}
}

func BenchmarkSerializeCompress(b *testing.B) {
	raw, _ := hex.DecodeString(UtxoRecord)
	var key UtxoKeyType
	var buf [0x100000]byte
	copy(key[:], raw[:])
	dat := raw[UtxoIdxLen:]
	rec := NewUtxoRecStatic(key, dat)
	SerializeC(rec, false, buf[:]) // serialize once to allocate the pools
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if SerializeC(rec, false, buf[:]) == nil {
			b.Fatal("Nil pointer returned")
		}
	}
}

func BenchmarkSerializeCompressWithAlloc(b *testing.B) {
	raw, _ := hex.DecodeString(UtxoRecord)
	var key UtxoKeyType
	copy(key[:], raw[:])
	dat := raw[UtxoIdxLen:]
	rec := NewUtxoRecStatic(key, dat)
	SerializeC(rec, false, nil) // serialize once to allocate the pools
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if SerializeC(rec, false, nil) == nil {
			b.Fatal("Nil pointer returned")
		}
	}
}
