/*
If you cannot compile this file, just replace it with the one from no_gcc/ folder
*/

package qdb

/*
#include <stdlib.h>
#include <strings.h>

void *alloc_ptr(void *c, size_t l) {
	void *ptr = malloc(l);
	memcpy(ptr, (const void *)c, l);
	return ptr;
}

*/
import "C"

import (
	"os"
	"unsafe"
	"reflect"
)

type data_ptr_t unsafe.Pointer

func (v *oneIdx) FreeData() {
	C.free(unsafe.Pointer(v.data))
	v.data = nil
}

func (v *oneIdx) Slice() (res []byte) {
	/*
	res = make([]byte, v.datlen)
	for i := range res {
		res[i] = *(*byte)(unsafe.Pointer(uintptr(v.data)+uintptr(i)))
	}
	*/
	res = *(*[]byte)(unsafe.Pointer(&reflect.SliceHeader{Data:uintptr(v.data), Len:int(v.datlen), Cap:int(v.datlen)}))
	return
}

func newIdx(v []byte, f uint32) (r *oneIdx) {
	r = new(oneIdx)
	r.data = data_ptr_t(C.alloc_ptr(unsafe.Pointer(&v[0]), C.size_t(len(v))))
	r.datlen = uint32(len(v))
	r.flags = f
	return
}

func (r *oneIdx) SetData(v []byte) {
	if r.data!=nil {
		C.free(unsafe.Pointer(r.data))
	}
	r.data = data_ptr_t(C.alloc_ptr(unsafe.Pointer(&v[0]), C.size_t(len(v))))
}

func (v *oneIdx) LoadData(f *os.File) {
	if v.data!=nil {
		C.free(unsafe.Pointer(v.data))
	}
	v.data = data_ptr_t(C.malloc(C.size_t(v.datlen)))
	f.Seek(int64(v.datpos), os.SEEK_SET)
	f.Read(*(*[]byte)(unsafe.Pointer(&reflect.SliceHeader{Data:uintptr(v.data), Len:int(v.datlen), Cap:int(v.datlen)})))
}
