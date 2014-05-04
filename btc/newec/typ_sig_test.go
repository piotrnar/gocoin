package newec

import (
	"strconv"
	"testing"
)

func TestSigRecover(t *testing.T) {
	var vs = [][6]string {
		{
			"6028b9e3a31c9e725fcbd7d5d16736aaaafcc9bf157dfb4be62bcbcf0969d488",
			"036d4a36fa235b8f9f815aa6f5457a607f956a71a035bf0970d8578bf218bb5a",
			"9cff3da1a4f86caf3683f865232c64992b5ed002af42b321b8d8a48420680487",
			"0",
			"56dc5df245955302893d8dda0677cc9865d8011bc678c7803a18b5f6faafec08",
			"54b5fbdcd8fac6468dac2de88fadce6414f5f3afbb103753e25161bef77705a6",
		},
		{
			"b470e02f834a3aaafa27bd2b49e07269e962a51410f364e9e195c31351a05e50",
			"560978aed76de9d5d781f87ed2068832ed545f2b21bf040654a2daff694c8b09",
			"9ce428d58e8e4caf619dc6fc7b2c2c28f0561654d1f80f322c038ad5e67ff8a6",
			"1",
			"15b7e7d00f024bffcd2e47524bb7b7d3a6b251e23a3a43191ed7f0a418d9a578",
			"bf29a25e2d1f32c5afb18b41ae60112723278a8af31275965a6ec1d95334e840",
		},
	}

	var sig sig_t
	var pubkey, exp ge_t
	var msg num_t

	for i := range vs {
		sig.r.set_hex(vs[i][0])
		sig.s.set_hex(vs[i][1])
		msg.set_hex(vs[i][2])
		rid, _ := strconv.ParseInt(vs[i][3], 10, 32)
		exp.x.set_hex(vs[i][4])
		exp.y.set_hex(vs[i][5])

		if sig.recover(&pubkey, &msg, int(rid)) {
			if exp.x.String()!=pubkey.x.String() {
				t.Error("x mismatch at vector", i)
			}
			if exp.y.String()!=pubkey.y.String() {
				t.Error("y mismatch at vector", i)
			}
		} else {
			t.Error("sig.recover fialed")
		}
	}
}
