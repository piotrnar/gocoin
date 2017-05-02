// +build linux

/*
If this file does not build and you don't know what to do, simply delete it and rebuild.
*/

package utxo

/*
#include <stdlib.h>
*/
import "C"

import (
	"unsafe"
	"reflect"
	"sync/atomic"
)


func gcc_malloc(le uint32) unsafe.Pointer {
	atomic.AddInt64(&ExtraMemoryConsumed, int64(le)+4)
	atomic.AddInt64(&ExtraMemoryAllocCnt, 1)
	ptr := unsafe.Pointer(C.malloc(C.size_t(le+4)))
	*((*uint32)(unsafe.Pointer(ptr))) = le
	return ptr
}

func gcc_free(ptr unsafe.Pointer) {
	atomic.AddInt64(&ExtraMemoryConsumed, -int64(gcc_len(ptr)+4))
	atomic.AddInt64(&ExtraMemoryAllocCnt, -1)
	C.free(unsafe.Pointer(ptr))
}

func gcc_malloc_and_copy(v []byte) unsafe.Pointer {
	ptr := gcc_malloc(uint32(len(v)))
	sl := *(*[]byte)(unsafe.Pointer(&reflect.SliceHeader{Data:uintptr(ptr)+4, Len:int(len(v)), Cap:int(len(v))}))
	copy(sl, v)
	return ptr
}

func gcc_len(ptr unsafe.Pointer) int {
	return int(*((*uint32)(ptr)))
}

func gcc_slice(ptr unsafe.Pointer) []byte {
	le := gcc_len(ptr)
	return *(*[]byte)(unsafe.Pointer(&reflect.SliceHeader{Data:uintptr(ptr)+4, Len:le, Cap:le}))
}

func init() {
	println("Using malloc() for UTXO memory bindings")
	malloc = gcc_malloc
	free = gcc_free
	malloc_and_copy = gcc_malloc_and_copy
	_len = gcc_len
	_slice = gcc_slice
}
