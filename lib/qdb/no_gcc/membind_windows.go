/*
Sometimes this does not quite work (just experimental)
*/

package qdb

import (
	"os"
	"unsafe"
	"reflect"
	"syscall"
	"sync/atomic"
)

type data_ptr_t unsafe.Pointer

var (
	kernel32 = syscall.NewLazyDLL("kernel32.dll")
	funcGlobalAlloc = kernel32.NewProc("GlobalAlloc")
	funcGlobalFree  = kernel32.NewProc("GlobalFree")
)

func HeapAlloc(le uint32) data_ptr_t {
	ptr, _, _ := funcGlobalAlloc.Call(0, uintptr(le))
	return data_ptr_t(ptr)
}

func HeapFree(ptr data_ptr_t) {
	funcGlobalFree.Call(uintptr(ptr))
}


func AllocPtr(v []byte) data_ptr_t {
	ptr := HeapAlloc(uint32(len(v)))
	sl := *(*[]byte)(unsafe.Pointer(&reflect.SliceHeader{Data:uintptr(ptr), Len:int(len(v)), Cap:int(len(v))}))
	copy(sl, v)
	return ptr
}


func (v *oneIdx) FreeData() {
	if v.data!=nil {
		HeapFree(v.data)
		atomic.AddInt64(&ExtraMemoryConsumed, -int64(v.datlen))
		atomic.AddInt64(&ExtraMemoryAllocCnt, -1)
	}
}

func (v *oneIdx) Slice() (res []byte) {
	res = *(*[]byte)(unsafe.Pointer(&reflect.SliceHeader{Data:uintptr(v.data), Len:int(v.datlen), Cap:int(v.datlen)}))
	return
}

func newIdx(v []byte, f uint32) (r *oneIdx) {
	r = new(oneIdx)
	r.data = AllocPtr(v)
	r.datlen = uint32(len(v))
	atomic.AddInt64(&ExtraMemoryConsumed, int64(r.datlen))
	atomic.AddInt64(&ExtraMemoryAllocCnt, 1)
	r.flags = f
	return
}

func (r *oneIdx) SetData(v []byte) {
	if r.data!=nil {
		panic("This should not happen")
	}
	r.data = AllocPtr(v)
	atomic.AddInt64(&ExtraMemoryConsumed, int64(r.datlen))
	atomic.AddInt64(&ExtraMemoryAllocCnt, 1)
}

func (v *oneIdx) LoadData(f *os.File) {
	if v.data!=nil {
		panic("This should not happen")
	}
	v.data = HeapAlloc(v.datlen)
	atomic.AddInt64(&ExtraMemoryConsumed, int64(v.datlen))
	atomic.AddInt64(&ExtraMemoryAllocCnt, 1)
	f.Seek(int64(v.datpos), os.SEEK_SET)
	f.Read(*(*[]byte)(unsafe.Pointer(&reflect.SliceHeader{Data:uintptr(v.data), Len:int(v.datlen), Cap:int(v.datlen)})))
}
