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
	atomic.AddInt64(&extraMemoryConsumed, int64(le)+24)
	atomic.AddInt64(&extraMemoryAllocCnt, 1)
	ptr := uintptr(C.malloc(C.size_t(le+24)))
	*(*reflect.SliceHeader)(unsafe.Pointer(ptr)) = reflect.SliceHeader{Data:ptr+24, Len:int(le), Cap:int(le)}
	return unsafe.Pointer(ptr)
}

func gcc_free(ptr unsafe.Pointer) {
	atomic.AddInt64(&extraMemoryConsumed, -int64(gcc_len(ptr)+24))
	atomic.AddInt64(&extraMemoryAllocCnt, -1)
	C.free(unsafe.Pointer(ptr))
}

func gcc_malloc_and_copy(v []byte) unsafe.Pointer {
	sl := gcc_malloc(uint32(len(v)))
	copy(_slice(sl), v)
	return sl
}

func gcc_len(ptr unsafe.Pointer) int {
	return len(*(*[]byte)(ptr))
}

func gcc_slice(ptr unsafe.Pointer) []byte {
	return *(*[]byte)(ptr)
}

func init() {
	println("Using malloc() for UTXO memory bindings")
	malloc = gcc_malloc
	free = gcc_free
	malloc_and_copy = gcc_malloc_and_copy
	_len = gcc_len
	_slice = gcc_slice
}
