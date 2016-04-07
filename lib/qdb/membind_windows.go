// +build windows

package qdb

import (
	"unsafe"
	"reflect"
	"syscall"
)

var (
	funcGlobalAlloc *syscall.Proc
	funcGlobalFree  *syscall.Proc
)

func win_HeapAlloc(le uint32) data_ptr_t {
	ptr, _, _ := funcGlobalAlloc.Call(0, uintptr(le))
	return data_ptr_t(ptr)
}

func win_HeapFree(ptr data_ptr_t) {
	funcGlobalFree.Call(uintptr(ptr))
}

func win_AllocPtr(v []byte) data_ptr_t {
	ptr := win_HeapAlloc(uint32(len(v)))
	sl := *(*[]byte)(unsafe.Pointer(&reflect.SliceHeader{Data:uintptr(ptr), Len:int(len(v)), Cap:int(len(v))}))
	copy(sl, v)
	return ptr
}


func init() {
	if membind_use_wrapper {
		return
	}
	dll, er := syscall.LoadDLL("kernel32.dll")
	if er!=nil {
		return
	}
	funcGlobalAlloc, _ = dll.FindProc("GlobalAlloc")
	funcGlobalFree, _ = dll.FindProc("GlobalFree")
	if funcGlobalAlloc==nil || funcGlobalFree==nil {
		return
	}
	println("Using kernel32.dll for qdb memory bindings")
	_HeapAlloc = win_HeapAlloc
	_HeapFree = win_HeapFree
	_AllocPtr = win_AllocPtr
	membind_use_wrapper = true
}
