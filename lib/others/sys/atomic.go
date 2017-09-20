package sys

import "sync/atomic"

type SyncBool struct {
	val int32
}

func (b *SyncBool) Get() bool {
	return atomic.LoadInt32(&b.val) != 0
}

func (b *SyncBool) Set(val bool) {
	if val {
		atomic.StoreInt32(&b.val, 1)
	} else {
		atomic.StoreInt32(&b.val, 0)
	}
}
