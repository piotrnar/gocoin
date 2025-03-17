package sys

import (
	"fmt"
	"runtime"
	"sync"
	"time"
)

type Dutex struct {
	ltime  time.Time
	file   string
	varstr string
	line   int
	lttot  time.Duration
	sync.Mutex
	mutint sync.Mutex
	locked bool
}

func (d *Dutex) Status() (s string) {
	d.mutint.Lock()
	if d.locked {
		s = "LOCKED from"
	} else {
		s = "last unlocked in"
	}
	s += fmt.Sprint(" ", d.file, ":", d.line, " ", time.Since(d.ltime).String(), " ago  (", d.lttot.String(), " total) -", d.varstr)
	d.mutint.Unlock()
	return
}

func (d *Dutex) fixfile() {
	var cnt int
	for i := len(d.file) - 1; i > 0; i-- {
		if d.file[i] == '/' {
			cnt++
			if cnt == 2 {
				d.file = d.file[i+1:]
				return
			}
		}
	}
}

func (d *Dutex) SetVar(v string) {
	d.mutint.Lock()
	d.varstr = v
	d.mutint.Unlock()
}

func (d *Dutex) Lock() {
	d.Mutex.Lock()
	d.mutint.Lock()
	d.locked = true
	_, d.file, d.line, _ = runtime.Caller(1)
	d.fixfile()
	d.ltime = time.Now()
	d.mutint.Unlock()
}

func (d *Dutex) Unlock() {
	d.mutint.Lock()
	ts := time.Since(d.ltime)
	if ts > time.Second {
		println(" >>> mutex locked from", d.file, "line", d.line, "took", ts.String(), "to unlock")
	}
	d.lttot += ts
	d.locked = false
	_, d.file, d.line, _ = runtime.Caller(1)
	d.fixfile()
	d.ltime = time.Now()
	d.mutint.Unlock()
	d.Mutex.Unlock()
}
