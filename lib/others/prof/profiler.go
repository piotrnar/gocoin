package prof

import (
	"sync"
	"time"
	"fmt"
	"sort"
)

var (
	ioprof sync.Once
	chpStart map[string]int64
	chpTotal map[string]int64
	start int64

	ProfilerDisabled bool = true
)

type oneval struct {
	name string
	tim int64
}
type sortif struct {
	val []oneval
}

func (i sortif) Len() int {
	return len(i.val)
}

func (x sortif) Less(i, j int) bool {
	return x.val[i].tim > x.val[j].tim
}

func (x sortif) Swap(i, j int) {
	x.val[i], x.val[j] = x.val[j], x.val[i]
}


func Sta(name string) {
	if ProfilerDisabled {
		return
	}
	_, ok := chpStart[name]
	if ok {
		panic(name+" already started")
	}
	chpStart[name] = time.Now().UnixNano()
}

func Sto(name string) {
	if ProfilerDisabled {
		return
	}
	tim, ok := chpStart[name]
	if !ok {
		panic(name+" not started")
	}
	delete(chpStart, name)
	del := time.Now().UnixNano()-tim
	tim, ok = chpTotal[name]
	if ok {
		chpTotal[name]  = tim+del
	} else {
		chpTotal[name]  = del
	}
}

func Stop() {
	ProfilerDisabled = true
	stop := time.Now().UnixNano() - start

	var mk sortif
	mk.val = make([]oneval, len(chpTotal))
	i := 0
	for k, v := range chpTotal {
	    mk.val[i].name = k
	    mk.val[i].tim = v
	    i++
	}
	sort.Sort(mk)

	for i := range mk.val {
		fmt.Printf("%40s : %8.3fs = %3d%%\n", mk.val[i].name,
			float64(mk.val[i].tim)/1e9, 100*mk.val[i].tim/stop)
	}
}

func Start() {
	chpStart = make(map[string]int64)
	chpTotal = make(map[string]int64)
	start = time.Now().UnixNano()
	ProfilerDisabled = false
}
