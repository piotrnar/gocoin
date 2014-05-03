package utils

import (
	"io/ioutil"
	"encoding/hex"
)



func is_hex_string(s []byte) (string) {
	var res string
	for i := range s {
		c := byte(s[i])
		if c<='9' && c>='0' || c<='f' && c>='a' || c<='F' && c>='A' {
			res += string(c)
		} else if c!=' ' && c!='\n' && c!='\r' && c!='\t' {
			return ""
		}
	}
	return res
}

// reads tx from the file or (if there is no such a file) decodes the text
func GetRawData(fn string) (dat []byte) {
	d, er := ioutil.ReadFile(fn)
	if er == nil {
		hexdump := is_hex_string(d)
		if len(hexdump)>=2 || (len(hexdump)&1)==1 {
			dat, _ = hex.DecodeString(hexdump)
		} else {
			dat = d
		}
	} else {
		dat, _ = hex.DecodeString(fn)
	}
	return
}
