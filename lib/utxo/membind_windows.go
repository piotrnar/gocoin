// +build windows

package utxo

import (
	"fmt"
	"unsafe"
	"reflect"
	"syscall"
	"sync/atomic"
)

var (
	funcGlobalAlloc *syscall.Proc
	funcGlobalFree  *syscall.Proc
)

func win_malloc(le uint32) unsafe.Pointer {
	atomic.AddInt64(&ExtraMemoryConsumed, int64(le)+24)
	atomic.AddInt64(&ExtraMemoryAllocCnt, 1)
	ptr, _, _ := funcGlobalAlloc.Call(0, uintptr(le+24))
	*(*reflect.SliceHeader)(unsafe.Pointer(ptr)) = reflect.SliceHeader{Data:ptr+24, Len:int(le), Cap:int(le)}
	return unsafe.Pointer(ptr)
}

func win_free(ptr unsafe.Pointer) {
	atomic.AddInt64(&ExtraMemoryConsumed, -int64(win_len(ptr)+24))
	atomic.AddInt64(&ExtraMemoryAllocCnt, -1)
	funcGlobalFree.Call(uintptr(ptr))
}

func win_malloc_and_copy(v []byte) unsafe.Pointer {
	ptr := win_malloc(uint32(len(v)))
	copy(win_slice(ptr), v)
	return ptr
}

func win_len(ptr unsafe.Pointer) int {
	return len(*(*[]byte)(ptr))
}

func win_slice(ptr unsafe.Pointer) []byte {
	return *(*[]byte)(ptr)
}

func init() {
	dll, er := syscall.LoadDLL("kernel32.dll")
	if er!=nil {
		return
	}
	funcGlobalAlloc, _ = dll.FindProc("GlobalAlloc")
	funcGlobalFree, _ = dll.FindProc("GlobalFree")
	fmt.Println("Using kernel32.dll for UTXO memory bindings")
	malloc = win_malloc
	free = win_free
	malloc_and_copy = win_malloc_and_copy
	_len = win_len
	_slice = win_slice
}
