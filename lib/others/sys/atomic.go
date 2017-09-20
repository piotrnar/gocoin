package sys

import "sync/atomic"

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

func (b *SyncBool) Store(val bool) {
	if val {
		b.Set()
	} else {
		b.Clr()
	}
}
