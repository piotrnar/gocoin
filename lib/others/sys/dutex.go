package sys

import (
	"fmt"
	"runtime"
	"sync"
	"time"
)

type Dutex struct {
	sync.Mutex
	file  string
	line  int
	ltime time.Time
}

func (d *Dutex) Status() string {
	return fmt.Sprint(d.file, ":", d.line, " locked ", time.Since(d.ltime).String(), " ago")
}

func (d *Dutex) Lock() {
	d.Mutex.Lock()
	_, d.file, d.line, _ = runtime.Caller(1)
	d.ltime = time.Now()
}

func (d *Dutex) Unlock() {
	d.line = -d.line
	if ts := time.Since(d.ltime); ts > time.Second {
		println(" >>> mutex locked from", d.file, "line", d.line, "took", ts.String(), "to unlock")
	}
	d.Mutex.Unlock()
}
