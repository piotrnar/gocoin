package utxo

import (
	"unsafe"
)

var (
	malloc func(le uint32) unsafe.Pointer = native_malloc
	free func(ptr unsafe.Pointer) = native_free
	malloc_and_copy func (v []byte) unsafe.Pointer = native_malloc_and_copy
	_len func(ptr unsafe.Pointer) int = native_len
	_slice func (ptr unsafe.Pointer) []byte = native_slice
)

var (
	ExtraMemoryConsumed int64  // if we are using the glibc memory manager
	ExtraMemoryAllocCnt int64  // if we are using the glibc memory manager
)

func native_malloc(le uint32) unsafe.Pointer {
	ptr := make([]byte, int(le))
	return unsafe.Pointer(&ptr)
}

func native_free(ptr unsafe.Pointer) {
}

func native_malloc_and_copy(v []byte) unsafe.Pointer {
	ptr := make([]byte, len(v))
	copy(ptr, v)
	return unsafe.Pointer(&ptr)
}

func native_len(ptr unsafe.Pointer) int {
	return len(native_slice(ptr))
}

func native_slice(ptr unsafe.Pointer) []byte {
	return *(*[]byte)(ptr)
}

func Slice(ptr unsafe.Pointer) []byte {
	return _slice(ptr)
}
