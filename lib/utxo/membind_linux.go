/*
If this file does not build and you don't know what to do, simply delete it and rebuild.
*/

package utxo

/*
#include <stdlib.h>
*/
import "C"

import (
	"fmt"
	"reflect"
	"sync/atomic"
	"unsafe"
)

func init() {
	MembindInit = func() {
		fmt.Println("Using malloc() and free() for UTXO records")

		malloc = func(le uint32) []byte {
			atomic.AddInt64(&extraMemoryConsumed, int64(le)+24)
			atomic.AddInt64(&extraMemoryAllocCnt, 1)
			ptr := uintptr(C.malloc(C.size_t(le + 24)))
			*(*reflect.SliceHeader)(unsafe.Pointer(ptr)) = reflect.SliceHeader{Data: ptr + 24, Len: int(le), Cap: int(le)}
			return *(*[]byte)(unsafe.Pointer(ptr))
		}

		free = func(ptr []byte) {
			atomic.AddInt64(&extraMemoryConsumed, -int64(len(ptr)+24))
			atomic.AddInt64(&extraMemoryAllocCnt, -1)
			C.free(unsafe.Pointer(uintptr(unsafe.Pointer(&ptr[0])) - 24))
		}

		malloc_and_copy = func (v []byte) []byte {
			sl := malloc(uint32(len(v)))
			copy(sl, v)
			return sl
		}
	}
}
