package utxo

import (
	"sync/atomic"
)

var (
	malloc func(le uint32) []byte = func(le uint32) []byte {
		return make([]byte, int(le))
	}

	free func([]byte) = func(v []byte) {
	}

	malloc_and_copy func (v []byte) []byte = func (v []byte) []byte {
		return v
	}

	MembindInit func() = func() {}
)

var (
	extraMemoryConsumed int64  // if we are using the glibc memory manager
	extraMemoryAllocCnt int64  // if we are using the glibc memory manager
)

func ExtraMemoryConsumed() int64 {
	return atomic.LoadInt64(&extraMemoryConsumed)
}

func ExtraMemoryAllocCnt() int64 {
	return atomic.LoadInt64(&extraMemoryAllocCnt)
}
