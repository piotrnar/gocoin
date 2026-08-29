package btc

import "testing"

// TestVLenNeverNegative makes sure VLen never returns a negative length for any
// input. A negative length used as a slice bound or a make() size panics the
// process on attacker-controlled data, so VLen must instead signal "invalid" by
// returning (0, 0) - the same sentinel it already uses for a truncated buffer.
func TestVLenNeverNegative(t *testing.T) {
	// 0xff-prefixed values with bit 63 set used to decode to a negative int.
	cases := [][]byte{
		{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff}, // 2^64-1
		{0xff, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x80}, // 2^63
		{0xff, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x80}, // 2^63+1
	}
	for i, c := range cases {
		le, siz := VLen(c)
		if le < 0 {
			t.Fatalf("case %d: VLen returned negative length %d", i, le)
		}
		if le != 0 || siz != 0 {
			t.Fatalf("case %d: expected (0,0) for out-of-range value, got (%d,%d)", i, le, siz)
		}
	}
}

// TestVLenValidValues guards against an over-aggressive check: legitimate
// var_ints must still decode to their exact value and byte size.
func TestVLenValidValues(t *testing.T) {
	cases := []struct {
		in  []byte
		le  int
		siz int
	}{
		{[]byte{0x00}, 0, 1},
		{[]byte{0xfc}, 0xfc, 1},
		{[]byte{0xfd, 0x00, 0x01}, 0x0100, 3},
		{[]byte{0xfe, 0xff, 0xff, 0xff, 0x7f}, 0x7fffffff, 5},
		{[]byte{0xff, 0xff, 0xff, 0xff, 0x7f, 0x00, 0x00, 0x00, 0x00}, 0x7fffffff, 9},
	}
	for i, tc := range cases {
		le, siz := VLen(tc.in)
		if le != tc.le || siz != tc.siz {
			t.Fatalf("case %d: got (%d,%d), want (%d,%d)", i, le, siz, tc.le, tc.siz)
		}
	}
}
