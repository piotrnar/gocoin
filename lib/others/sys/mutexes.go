package sys

import (
	"fmt"
	"runtime"
	"sync"
)

type Mutex struct {
	sync.Mutex
	locked bool
	line   int
	file   string
}

func (m *Mutex) Lock() {
	m.Mutex.Lock()
	m.locked = true
	_, m.file, m.line, _ = runtime.Caller(1)
}

func (m *Mutex) Unlock() {
	//_, m.file, m.line, _ = runtime.Caller(1)
	m.locked = false
	m.Mutex.Unlock()
}

func (m *Mutex) String() string {
	if m.locked {
		return fmt.Sprint("lock from ", m.file, ":", m.line)
	}
	return "unlocked"
	//return fmt.Sprint("unlocked in ", m.file, ":", m.line)
}
