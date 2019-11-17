package bech32

import (
	"bytes"
)

func bech32_polymod_step(pre uint32) uint32 {
	b := uint32(pre >> 25)
	return ((pre & 0x1FFFFFF) << 5) ^
		(-((b >> 0) & 1) & 0x3b6a57b2) ^
		(-((b >> 1) & 1) & 0x26508e6d) ^
		(-((b >> 2) & 1) & 0x1ea119fa) ^
		(-((b >> 3) & 1) & 0x3d4233dd) ^
		(-((b >> 4) & 1) & 0x2a1462b3)
}

const (
	charset = "qpzry9x8gf2tvdw0s3jn54khce6mua7l"
)

var (
	charset_rev = [128]byte{
		99, 99, 99, 99, 99, 99, 99, 99, 99, 99, 99, 99, 99, 99, 99, 99,
		99, 99, 99, 99, 99, 99, 99, 99, 99, 99, 99, 99, 99, 99, 99, 99,
		99, 99, 99, 99, 99, 99, 99, 99, 99, 99, 99, 99, 99, 99, 99, 99,
		15, 99, 10, 17, 21, 20, 26, 30, 7, 5, 99, 99, 99, 99, 99, 99,
		99, 29, 99, 24, 13, 25, 9, 8, 23, 99, 18, 22, 31, 27, 19, 99,
		1, 0, 3, 16, 11, 28, 12, 14, 6, 4, 2, 99, 99, 99, 99, 99,
		99, 29, 99, 24, 13, 25, 9, 8, 23, 99, 18, 22, 31, 27, 19, 99,
		1, 0, 3, 16, 11, 28, 12, 14, 6, 4, 2, 99, 99, 99, 99, 99}
)

// Encode returns an empty string on error.
func Encode(hrp string, data []byte) string {
	var chk uint32 = 1
	var i int
	output := new(bytes.Buffer)
	for i = range hrp {
		ch := int(hrp[i])
		if ch < 33 || ch > 126 {
			return ""
		}

		if ch >= 'A' && ch <= 'Z' {
			return ""
		}
		chk = bech32_polymod_step(chk) ^ (uint32(ch) >> 5)
		i++
	}
	if i+7+len(data) > 90 {
		return ""
	}
	chk = bech32_polymod_step(chk)
	for i := range hrp {
		tmp := hrp[i]
		chk = bech32_polymod_step(chk) ^ uint32(tmp&0x1f)
		output.WriteByte(tmp)
	}
	output.WriteByte('1')

	for i = range data {
		if (data[i] >> 5) != 0 {
			return ""
		}
		chk = bech32_polymod_step(chk) ^ uint32(data[i])
		output.WriteByte(charset[data[i]])
	}
	for i = 0; i < 6; i++ {
		chk = bech32_polymod_step(chk)
	}
	chk ^= 1
	for i = 0; i < 6; i++ {
		output.WriteByte(charset[(chk>>uint((5-i)*5))&0x1f])
	}
	return string(output.Bytes())
}

// Decode returns ("", nil) on error.
func Decode(input string) (res_hrp string, res_data []byte) {
	var chk uint32 = 1
	var i, data_len, hrp_len int
	var have_lower, have_upper bool
	if len(input) < 8 || len(input) > 90 {
		return
	}
	for data_len < len(input) && input[(len(input)-1)-data_len] != '1' {
		data_len++
	}
	hrp_len = len(input) - (1 + data_len)
	if hrp_len < 1 || data_len < 6 {
		return
	}
	data_len -= 6
	hrp := make([]byte, hrp_len)
	data := make([]byte, data_len)
	for i = 0; i < hrp_len; i++ {
		ch := input[i]
		if ch < 33 || ch > 126 {
			return
		}
		if ch >= 'a' && ch <= 'z' {
			have_lower = true
		} else if ch >= 'A' && ch <= 'Z' {
			have_upper = true
			ch = (ch - 'A') + 'a'
		}
		hrp[i] = ch
		chk = bech32_polymod_step(chk) ^ uint32(ch>>5)
	}
	chk = bech32_polymod_step(chk)
	for i = 0; i < hrp_len; i++ {
		chk = bech32_polymod_step(chk) ^ uint32(input[i]&0x1f)
	}
	i++
	for i < len(input) {
		if (input[i] & 0x80) != 0 {
			return
		}
		v := charset_rev[(input[i])]
		if v > 31 {
			return
		}
		if input[i] >= 'a' && input[i] <= 'z' {
			have_lower = true
		}
		if input[i] >= 'A' && input[i] <= 'Z' {
			have_upper = true
		}
		chk = bech32_polymod_step(chk) ^ uint32(v)
		if i+6 < len(input) {
			data[i-(1+hrp_len)] = v
		}
		i++
	}
	if have_lower && have_upper {
		return
	}
	if chk == 1 {
		res_hrp = string(hrp)
		res_data = data
	}
	return
}
