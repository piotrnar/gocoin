package sys

import (
	"fmt"
	"sync/atomic"
)

type SyncBool struct {
	val int32
}

func (b *SyncBool) Get() bool {
	return atomic.LoadInt32(&b.val) != 0
}

func (b *SyncBool) Set() {
	atomic.StoreInt32(&b.val, 1)
}

func (b *SyncBool) Clr() {
	atomic.StoreInt32(&b.val, 0)
}

func (b *SyncBool) MarshalText() (text []byte, err error) {
	return []byte(fmt.Sprint(b.Get())), nil
}

func (b *SyncBool) Store(val bool) {
	if val {
		b.Set()
	} else {
		b.Clr()
	}
}


type SyncInt struct {
	val int64
}

func (b *SyncInt) Get() int {
	return int(atomic.LoadInt64(&b.val))
}

func (b *SyncInt) Store(val int) {
	atomic.StoreInt64(&b.val, int64(val))
}

func (b *SyncInt) MarshalText() (text []byte, err error) {
	return []byte(fmt.Sprint(b.Get())), nil
}
