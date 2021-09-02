package bech32

import (
	"bytes"
	"errors"
)

// convert_bits returns nil on error.
func convert_bits(outbits uint, in []byte, inbits uint, pad bool) []byte {
	var val uint32
	var bits uint
	maxv := uint32(1<<outbits) - 1
	out := new(bytes.Buffer)
	for inx := range in {
		val = (val << inbits) | uint32(in[inx])
		bits += inbits
		for bits >= outbits {
			bits -= outbits
			out.WriteByte(byte((val >> bits) & maxv))
		}
	}
	if pad {
		if bits != 0 {
			out.WriteByte(byte((val << (outbits - bits)) & maxv))
		}
	} else if ((val<<(outbits-bits))&maxv) != 0 || bits >= inbits {
		return nil
	}
	return out.Bytes()
}

// SegwitEncode returns an empty string on error.
func SegwitEncode(hrp string, witver int, witprog []byte) string {
	if witver > 16 {
		return ""
	}
	if witver == 0 && len(witprog) != 20 && len(witprog) != 32 {
		return ""
	}
	if len(witprog) < 2 || len(witprog) > 40 {
		return ""
	}
	return Encode(hrp, append([]byte{byte(witver)}, convert_bits(5, witprog, 8, true)...), witver > 0)
}

// SegwitDecode returns (0, nil) on error.
func SegwitDecode(hrp, addr string) (witver int, witdata []byte, er error) {
	hrp_actual, data, bech32m := Decode(addr)
	if hrp_actual == "" || len(data) == 0 || len(data) > 65 {
		er = errors.New("BECH32 decode error")
		return
	}
	if hrp != hrp_actual {
		er = errors.New("HRP mismatch")
		return
	}
	if data[0] > 16 {
		er = errors.New("WITNESS Version too high")
		return
	}
	if data[0] == 0 && bech32m {
		er = errors.New("WITNESS using M for Version 0")
		return
	}
	if data[0] != 0 && !bech32m {
		er = errors.New("WITNESS not using M when needed")
		return
	}
	witdata = convert_bits(8, data[1:], 5, false)
	if witdata == nil {
		er = errors.New("ERROR from convert_bits")
		return
	}
	if len(witdata) < 2 || len(witdata) > 40 {
		er = errors.New("WITNESS data length error")
		witdata = nil
		return
	}
	if data[0] == 0 && len(witdata) != 20 && len(witdata) != 32 {
		er = errors.New("WITNESS Version 0 data length error")
		witdata = nil
		return
	}
	witver = int(data[0])
	return
}
