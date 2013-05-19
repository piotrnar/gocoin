package main

import (
	"testing"
)

func TestParseAmount(t *testing.T) {
	var tv = []struct {
		af string
		ai uint64
	} {
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
	}
	for i := range tv {
		res := ParseAmount(tv[i].af)
		if res!=tv[i].ai {
			t.Error("Mismatch at index", i, res, tv[i].ai)
		}
	}
}
