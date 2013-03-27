package btc

import (
	"os"
)

type scrStack struct {
	data [][]byte
}

func (s *scrStack) push(d []byte) {
	s.data = append(s.data, d)
}

func (s *scrStack) top(idx int) ([]byte) {
	l := len(s.data)
	return s.data[l+idx]
}


func (s *scrStack) pop() (d []byte) {
	l := len(s.data)
	if l==0 {
		println("stack is empty")
		os.Exit(1)
	}
	d = s.data[l-1]
	s.data = s.data[:l-1]
	return
}

func (s *scrStack) size() int {
	return len(s.data)
}

