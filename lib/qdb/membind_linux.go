// +build linux

/*
If this file does not build and you don't know what to do, simply delete it and rebuild.
*/

package qdb

/*
#include <stdlib.h>
#include <string.h>

void *alloc_ptr(void *c, unsigned long l) {
	void *ptr = malloc(l);
	memcpy(ptr, c, l);
	return ptr;
}

void *my_alloc(unsigned long l) {
	return malloc(l);
}

*/
import "C"

import (
	"unsafe"
)


func gcc_HeapAlloc(le uint32) data_ptr_t {
	return data_ptr_t(C.my_alloc(C.ulong(le)))
}

func gcc_HeapFree(ptr data_ptr_t) {
	C.free(unsafe.Pointer(ptr))
}

func gcc_AllocPtr(v []byte) data_ptr_t {
	ptr := unsafe.Pointer(&v[0]) // see https://github.com/golang/go/issues/15172
	return data_ptr_t(C.alloc_ptr(ptr, C.ulong(len(v))))
}

func init() {
	if membind_use_wrapper {
		panic("Another wrapper already initialized")
	}
	println("Using CGO for QDB memory. Remove file qdb/membind_linux.go if you encounter issues.")
	_HeapAlloc = gcc_HeapAlloc
	_HeapFree = gcc_HeapFree
	_AllocPtr = gcc_AllocPtr
	membind_use_wrapper = true
}
