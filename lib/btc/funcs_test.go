package btc

import (
	"bufio"
	"bytes"
	"testing"
)

var var_int_tvs = []uint64{
	0x0,
	0x7f, 0x80,
	0x407f, 0x4080,
	0x20407f, 0x204080,
	0x1020407f, 0x10204080,
	0x81020407f, 0x810204080,
	0xffffffffffffffff,
}

func TestParseAmount(t *testing.T) {
	var tv = []struct {
		af string
		ai uint64
	}{
		{"84.3449", 8434490000},
		{"84.3448", 8434480000},
		{"84.3447", 8434470000},
		{"84.3446", 8434460000},
		{"84.3445", 8434450000},
		{"84.3444", 8434440000},
		{"84.3443", 8434430000},
		{"84.3442", 8434420000},
		{"84.3441", 8434410000},
		{"84.3440", 8434400000},
		{"84.3439", 8434390000},
		{"0.99999990", 99999990},
		{"0.99999991", 99999991},
		{"0.99999992", 99999992},
		{"0.99999993", 99999993},
		{"0.99999994", 99999994},
		{"0.99999995", 99999995},
		{"0.99999996", 99999996},
		{"0.99999997", 99999997},
		{"0.99999998", 99999998},
		{"0.99999999", 99999999},
		{"1.00000001", 100000001},
		{"1.00000002", 100000002},
		{"1.00000003", 100000003},
		{"1.00000004", 100000004},
		{"1.00000005", 100000005},
		{"1.00000006", 100000006},
		{"1.00000007", 100000007},
		{"1000000.0", 100000000000000},
		{"100000.0", 10000000000000},
		{"10000.0", 1000000000000},
		{"1000.0", 100000000000},
		{"100.0", 10000000000},
		{"10.0", 1000000000},
		{"1.0", 100000000},
		{"0.1", 10000000},
		{"0.01", 1000000},
		{"0.001", 100000},
		{"0.00001", 1000},
		{"0.000001", 100},
		{"0.0000001", 10},
		{"0.00000001", 1},
	}
	for i := range tv {
		res, _ := StringToSatoshis(tv[i].af)
		if res != tv[i].ai {
			t.Error("Mismatch at index", i, tv[i].af, res, tv[i].ai)
		}
	}
}

func TestVarInt(t *testing.T) {
	for i, val := range var_int_tvs {
		buf := new(bytes.Buffer)
		wr := bufio.NewWriter(buf)
		WriteVarInt(wr, val)
		wr.Flush()
		var_int := buf.Bytes()
		n, er := ReadVarInt(bufio.NewReader(bytes.NewBuffer(var_int)))
		if er != nil {
			t.Error("ReadVarInt error at index", i, val, er.Error())
			continue
		}
		if n != val {
			t.Error("Mismatch at index", i, var_int_tvs[i])
		}
	}
}
func BenchmarkVarIntRead(b *testing.B) {
	recs := make([][]byte, len(var_int_tvs))
	for i, v := range var_int_tvs {
		bts := new(bytes.Buffer)
		wr := bufio.NewWriter(bts)
		WriteVarInt(wr, v)
		wr.Flush()
		recs[i] = bts.Bytes()

	}

	rd := bufio.NewReader(nil)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, rec := range recs {
			rd.Reset(bytes.NewBuffer(rec))
			ReadVarInt(rd)
		}
	}
}
func BenchmarkVarIntWrite(b *testing.B) {
	wr := bufio.NewWriter(nil)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		wr.Reset(new(bytes.Buffer))
		for _, v := range var_int_tvs {
			WriteVarInt(wr, v)
		}
		wr.Flush()
	}
}

func BenchmarkVLenRead(b *testing.B) {
	recs := make([][]byte, len(var_int_tvs))
	for i, v := range var_int_tvs {
		bts := new(bytes.Buffer)
		WriteVlen(bts, v)
		recs[i] = bts.Bytes()
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, rec := range recs {
			ReadVLen(bytes.NewBuffer(rec))
		}
	}
}
func BenchmarkVLenWrite(b *testing.B) {
	wr := new(bytes.Buffer)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, v := range var_int_tvs {
			wr.Reset()
			WriteVlen(wr, v)
		}
	}
}
