package script

import (
	"fmt"
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

func (s *scrStack) pushInt(val int64) {
	var negative bool

	if val<0 {
		negative = true
		val = -val
	}
	var d []byte

	if val!=0 {
		for val!=0 {
			d = append(d, byte(val))
			val >>= 8
		}

		if negative {
			if (d[len(d)-1]&0x80) != 0 {
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


func bts2int(d []byte) (res int64) {
	if len(d) > nMaxNumSize {
		panic("Int on the stack is too long")
		// Make sure this panic is captured in evalScript (cause the script to fail, not crash)
	}

	if len(d)==0 {
		return
	}

	var i int
	for i<len(d)-1 {
		res |= int64(d[i]) << uint(i*8)
		i++
	}

	if (d[i]&0x80)!=0 {
		res |= int64(d[i]&0x7f) << uint(i*8)
		res = -res
	} else {
		res |= int64(d[i]) << uint(i*8)
	}

	return
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
	return (d[len(d)-1]&0x7f) != 0 // -0 (0x80) is also false (I hope..)
}


func (s *scrStack) popInt() int64 {
	return bts2int(s.pop())
}

func (s *scrStack) popBool() bool {
	return bts2bool(s.pop())
}

func (s *scrStack) top(idx int) (d []byte) {
	return s.data[len(s.data)+idx]
}

func (s *scrStack) topInt(idx int) int64 {
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

func (s *scrStack) nofalse() bool {
	for i := range s.data {
		if !bts2bool(s.data[i]) {
			return false
		}
	}
	return true
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
