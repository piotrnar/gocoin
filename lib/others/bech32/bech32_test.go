package bech32

import (
	"strings"
	"testing"
)

var (
	valid_checksum = []string{
		"A12UEL5L",
		"an83characterlonghumanreadablepartthatcontainsthenumber1andtheexcludedcharactersbio1tt5tgs",
		"abcdef1qpzry9x8gf2tvdw0s3jn54khce6mua7lmqqqxw",
		"11qqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqc8247j",
		"split1checkupstagehandshakeupstreamerranterredcaperred2y9e3w"}

	invalid_checksum = []string{
		" 1nwldj5",
		"\x7f1axkwrx",
		"an84characterslonghumanreadablepartthatcontainsthenumber1andtheexcludedcharactersbio1569pvx",
		"pzry9x0s0muk",
		"1pzry9x0s0muk",
		"x1b4n0q5v",
		"li1dgmt3",
		"de1lg7wt\xff"}
)

func TestValidChecksum(t *testing.T) {
	for _, s := range valid_checksum {
		hrp, data := Decode(s)
		if data == nil || hrp == "" {
			t.Error("Decode fails: ", s)
		} else {
			rebuild := Encode(hrp, data)
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

func TestInvalidChecksum(t *testing.T) {
	for _, s := range invalid_checksum {
		hrp, data := Decode(s)
		if data != nil || hrp != "" {
			t.Error("Decode succeeds on invalid string: ", s)
		}
	}
}
