package utxo

import (
	"fmt"
	"reflect"
	"sync/atomic"
	"syscall"
	"unsafe"
)


func init() {
	MembindInit = func() {
		var (
			hHeap             uintptr
			funcHeapAllocAddr uintptr
			funcHeapFreeAddr  uintptr
		)

		dll, er := syscall.LoadDLL("kernel32.dll")
		if er != nil {
			return
		}
		fun, _ := dll.FindProc("GetProcessHeap")
		hHeap, _, _ = fun.Call()

		fun, _ = dll.FindProc("HeapAlloc")
		funcHeapAllocAddr = fun.Addr()

		fun, _ = dll.FindProc("HeapFree")
		funcHeapFreeAddr = fun.Addr()

		fmt.Println("Using kernel32.dll for UTXO records")
		malloc = func(le uint32) []byte {
			atomic.AddInt64(&extraMemoryConsumed, int64(le)+24)
			atomic.AddInt64(&extraMemoryAllocCnt, 1)
			ptr, _, _ := syscall.Syscall(funcHeapAllocAddr, 3, hHeap, 0, uintptr(le+24))
			*(*reflect.SliceHeader)(unsafe.Pointer(ptr)) = reflect.SliceHeader{Data: ptr + 24, Len: int(le), Cap: int(le)}
			return *(*[]byte)(unsafe.Pointer(ptr))
		}

		free = func(ptr []byte) {
			atomic.AddInt64(&extraMemoryConsumed, -int64(len(ptr)+24))
			atomic.AddInt64(&extraMemoryAllocCnt, -1)
			syscall.Syscall(funcHeapFreeAddr, 3, hHeap, 0, uintptr(unsafe.Pointer(&ptr[0]))-24)
		}

		malloc_and_copy = func(v []byte) []byte {
			ptr := malloc(uint32(len(v)))
			copy(ptr, v)
			return ptr
		}
	}
}
