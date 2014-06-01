package qdb

import "os"

type data_ptr_t []byte

func (v *oneIdx) FreeData() {
	v.data = nil
}

func (v *oneIdx) Slice() (res []byte) {
	return v.data
}

func newIdx(v []byte, f uint32) (r *oneIdx) {
	r = new(oneIdx)
	r.data = v
	r.datlen = uint32(len(v))
	r.flags = f
	return
}

func (r *oneIdx) SetData(v []byte) {
	r.data = v
}

func (v *oneIdx) LoadData(f *os.File) {
	v.data = make([]byte, int(v.datlen))
	f.Seek(int64(v.datpos), os.SEEK_SET)
	f.Read(v.data)
}
