package bech32

import (
	"strings"
	"testing"
)

var (
	valid_checksum_bech32 = []string{
		"A12UEL5L",
		"a12uel5l",
		"an83characterlonghumanreadablepartthatcontainsthenumber1andtheexcludedcharactersbio1tt5tgs",
		"abcdef1qpzry9x8gf2tvdw0s3jn54khce6mua7lmqqqxw",
		"11qqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqc8247j",
		"split1checkupstagehandshakeupstreamerranterredcaperred2y9e3w",
		"?1ezyfcl",
	}

	valid_checksum_bech32m = []string{
		"A1LQFN3A",
		"a1lqfn3a",
		"an83characterlonghumanreadablepartthatcontainsthetheexcludedcharactersbioandnumber11sg7hg6",
		"abcdef1l7aum6echk45nj3s0wdvt2fg8x9yrzpqzd3ryx",
		"11llllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllllludsr8",
		"split1checkupstagehandshakeupstreamerranterredcaperredlc445v",
		"?1v759aa",
	}

	invalid_checksum_bech32 = []string{
		" 1nwldj5",
		"\x7f1axkwrx",
		"\x801eym55h",
		"an84characterslonghumanreadablepartthatcontainsthenumber1andtheexcludedcharactersbio1569pvx",
		"pzry9x0s0muk",
		"1pzry9x0s0muk",
		"x1b4n0q5v",
		"li1dgmt3",
		"de1lg7wt\xff",
		"A1G7SGD8",
		"10a06t8",
		"1qzzfhee",
	}

	invalid_checksum_bech32m = []string{
		" 1xj0phk",
		"\x7F1g6xzxy",
		"\x801vctc34",
		"an84characterslonghumanreadablepartthatcontainsthetheexcludedcharactersbioandnumber11d6pts4",
		"qyrz8wqd2c9m",
		"1qyrz8wqd2c9m",
		"y1b0jsk6g",
		"lt1igcx5c0",
		"in1muywd",
		"mm1crxm3i",
		"au1s5cgom",
		"M1VUXWEZ",
		"16plkw9",
		"1p2gdwpf",
	}
)

func TestValidChecksumB32(t *testing.T) {
	for _, s := range valid_checksum_bech32 {
		hrp, data, bech32m := Decode(s)
		if data == nil || hrp == "" || bech32m {
			t.Error("Decode fails: ", s)
		} else {
			rebuild := Encode(hrp, data, false)
			if rebuild == "" {
				t.Error("Encode fails: ", s)
			} else {
				if !strings.EqualFold(s, rebuild) {
					t.Error("Encode produces incorrect result: ", s)
				}
			}
		}
	}
}

func TestValidChecksumB32m(t *testing.T) {
	for _, s := range valid_checksum_bech32m {
		hrp, data, bech32m := Decode(s)
		if data == nil || hrp == "" || !bech32m {
			t.Error("Decode fails: ", s)
		} else {
			rebuild := Encode(hrp, data, true)
			if rebuild == "" {
				t.Error("Encode fails: ", s)
			} else {
				if !strings.EqualFold(s, rebuild) {
					t.Error("Encode produces incorrect result: ", s)
				}
			}
		}
	}
}

func TestInvalidChecksumB32(t *testing.T) {
	for _, s := range invalid_checksum_bech32 {
		hrp, data, bech32m := Decode(s)
		if data != nil || hrp != "" || bech32m {
			t.Error("Decode succeeds on invalid string: ", s)
		}
	}
}

func TestInvalidChecksumB32m(t *testing.T) {
	for _, s := range invalid_checksum_bech32m {
		hrp, data, bech32m := Decode(s)
		if data != nil || hrp != "" || bech32m {
			t.Error("Decode succeeds on invalid string: ", s)
		}
	}
}
