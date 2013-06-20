package btc

import (
	"fmt"
	"math/big"
	"encoding/hex"
)

const nMaxNumSize = 4

type scrStack struct {
	data [][]byte
}

func (s *scrStack) push(d []byte) {
	s.data = append(s.data, d)
}

func (s *scrStack) pushBool(v bool) {
	if v {
		s.data = append(s.data, []byte{1})
	} else {
		s.data = append(s.data, []byte{})
	}
}

func (s *scrStack) pushInt(val *big.Int) {
	var negative bool

	if val.Sign()<0 {
		negative = true
		val.Neg(val)
	}
	bigend := val.Bytes()
	var d []byte

	if len(bigend)!=0 {
		d = make([]byte, len(bigend))
		for i := range bigend {
			d[len(bigend)-i-1] = bigend[i]
		}

		if negative {
			if (bigend[0]&0x80) != 0 {
				d = append(d, 0x80)
			} else {
				d[len(d)-1] |= 0x80
			}
		} else if (d[len(d)-1]&0x80) != 0 {
			d = append(d, 0x00)
		}
	}

	s.data = append(s.data, d)
}

// Converts a little endian, BTC format, integer into big.Int
func bts2int(d []byte) *big.Int {
	if len(d) == 0 {
		return new(big.Int)
	}

	if len(d) > nMaxNumSize {
		panic("BigInt from the stack overflow")
	}

	// convert little endian to big endian
	bigend := make([]byte, len(d))
	for i := range d {
		bigend[len(d)-i-1] = d[i]
	}

	// process the sign bit
	if (bigend[0]&0x80) != 0 {
		bigend[0] &= 0x7f // negative value - remove the sign bit
		return new(big.Int).Neg(new(big.Int).SetBytes(bigend))
	} else {
		return new(big.Int).SetBytes(bigend)
	}
}


func bts2bool(d []byte) bool {
	if len(d)==0 {
		return false
	}
	for i:=0; i<len(d)-1; i++ {
		if d[i]!=0 {
			return true
		}
	}
	return d[len(d)-1]!=0x80 // -0 is also false (I hope..)
}


func (s *scrStack) popInt() *big.Int {
	return bts2int(s.pop())
}

func (s *scrStack) popBool() bool {
	return bts2bool(s.pop())
}

func (s *scrStack) top(idx int) (d []byte) {
	return s.data[len(s.data)+idx]
}

func (s *scrStack) topInt(idx int) *big.Int {
	return bts2int(s.data[len(s.data)+idx])
}

func (s *scrStack) topBool(idx int) bool {
	return bts2bool(s.data[len(s.data)+idx])
}

func (s *scrStack) pop() (d []byte) {
	l := len(s.data)
	if l==0 {
		panic("stack is empty")
	}
	d = s.data[l-1]
	s.data = s.data[:l-1]
	return
}

func (s *scrStack) empties() (res int) {
	for i := range s.data {
		if len(s.data[i])==0 {
			res++
		}
	}
	return
}

func (s *scrStack) size() int {
	return len(s.data)
}

func (s *scrStack) print() {
	fmt.Println(len(s.data), "elements on stack:")
	for i := range s.data {
		fmt.Printf("%3d: len=%d, data:%s\n", i, len(s.data[i]), hex.EncodeToString(s.data[i]))
	}
}
