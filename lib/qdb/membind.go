package qdb

import (
	"os"
	"unsafe"
	"reflect"
	"sync/atomic"
)

var (
	membind_use_wrapper bool
	_HeapAlloc func(le uint32) data_ptr_t
	_HeapFree func(ptr data_ptr_t)
	_AllocPtr func(v []byte) data_ptr_t
)


type data_ptr_t unsafe.Pointer

func (v *oneIdx) FreeData() {
	if membind_use_wrapper {
		_HeapFree(v.data)
		atomic.AddInt64(&ExtraMemoryConsumed, -int64(v.datlen))
		atomic.AddInt64(&ExtraMemoryAllocCnt, -1)
	}
	v.data = nil
}

func (v *oneIdx) Slice() (res []byte) {
	if membind_use_wrapper {
		res = *(*[]byte)(unsafe.Pointer(&reflect.SliceHeader{Data:uintptr(v.data), Len:int(v.datlen), Cap:int(v.datlen)}))
	} else {
		res = *(*[]byte)(v.data)
	}
	return
}

func newIdx(v []byte, f uint32) (r *oneIdx) {
	r = new(oneIdx)
	r.datlen = uint32(len(v))
	if membind_use_wrapper {
		r.data = _AllocPtr(v)
		atomic.AddInt64(&ExtraMemoryConsumed, int64(r.datlen))
		atomic.AddInt64(&ExtraMemoryAllocCnt, 1)
	} else {
		r.data = data_ptr_t(&v)
	}
	r.flags = f
	return
}

func (r *oneIdx) SetData(v []byte) {
	if membind_use_wrapper {
		r.data = _AllocPtr(v)
		atomic.AddInt64(&ExtraMemoryConsumed, int64(r.datlen))
		atomic.AddInt64(&ExtraMemoryAllocCnt, 1)
	} else {
		r.data = data_ptr_t(&v)
	}
}

func (v *oneIdx) LoadData(f *os.File) {
	if membind_use_wrapper {
		v.data = _HeapAlloc(v.datlen)
		atomic.AddInt64(&ExtraMemoryConsumed, int64(v.datlen))
		atomic.AddInt64(&ExtraMemoryAllocCnt, 1)
		f.Seek(int64(v.datpos), os.SEEK_SET)
		f.Read(*(*[]byte)(unsafe.Pointer(&reflect.SliceHeader{Data:uintptr(v.data), Len:int(v.datlen), Cap:int(v.datlen)})))
	} else {
		ptr := make([]byte, int(v.datlen))
		v.data = data_ptr_t(&ptr)
		f.Seek(int64(v.datpos), os.SEEK_SET)
		f.Read(ptr)
	}
}
