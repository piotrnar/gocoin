// +build linux

/*
If this file does not build and you don't know what to do, simply delete it and rebuild.
*/

package utxo

/*
#include <stdlib.h>
#include <string.h>

static void *alloc_ptr(void *c, unsigned long l) {
	void *ptr = malloc(l);
	memcpy(ptr, c, l);
	return ptr;
}

static void *my_alloc(unsigned long l) {
	return malloc(l);
}

*/
import "C"

import (
	"unsafe"
	"reflect"
)


func gcc_malloc(le uint32) unsafe.Pointer {
	return unsafe.Pointer(C.my_alloc(C.ulong(le)))
}

func gcc_free(ptr unsafe.Pointer) {
	C.free(unsafe.Pointer(ptr))
}

func gcc_malloc_and_copy(v []byte) unsafe.Pointer {
	ptr := unsafe.Pointer(&v[0]) // see https://github.com/golang/go/issues/15172
	return unsafe.Pointer(C.alloc_ptr(ptr, C.ulong(len(v))))
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
