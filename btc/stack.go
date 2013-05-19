package btc

import (
	"fmt"
	"encoding/hex"
)

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
	if val==0 {
		s.data = append(s.data, []byte{})
		return
	}
	neg := (val<0)
	if neg {
		val = -val
	}
	var buf = []byte{byte(val)}
	val >>= 8
	for val != 0 {
		buf = append(buf, byte(val))
		val >>= 8
	}
	if neg {
		if (buf[len(buf)-1]&0x80)==0 {
			buf[len(buf)-1] |= 0x80
		} else {
			buf = append(buf, 0x80)
		}
	}
	s.data = append(s.data, buf)
}



func bts2int(d []byte) int64 {
	//println("bts2int", hex.EncodeToString(d), "...")
	if len(d) == 0 {
		return 0
	} else if len(d) > 8 {
		println("Int too long", len(d))
	}
	var neg bool
	var res uint64
	for i := range d {
		if i==len(d)-1 {
			neg = (d[i]&0x80) != 0
			res |= ( uint64(d[i]&0x7f) << uint64(8*i) )
		} else {
			res |= ( uint64(d[i]) << uint64(8*i) )
		}
	}
	if neg {
		//println("... neg ", res)
		return -int64(res)
	} else {
		//println("... +", res)
		return int64(res)
	}
}


func (s *scrStack) popInt() int64 {
	return bts2int(s.pop())
}

func (s *scrStack) popBool() bool {
	d := s.pop()
	for i := range d {
		if d[i]!=0 {
			return true
		}
	}
	return false
}

func (s *scrStack) top(idx int) (d []byte) {
	return s.data[len(s.data)+idx]
}

func (s *scrStack) topInt(idx int) int64 {
	return bts2int(s.data[len(s.data)+idx])
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