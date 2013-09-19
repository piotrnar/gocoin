package newec

/*
void bytes2bn(void *out, void *bytes, int len);
*/
import "C"
import "unsafe"

func (r *ecdsa_sig_t) get_bns() unsafe.Pointer {
	res := make([]byte, 64)
	dat := r.r.Bytes()
	C.bytes2bn(unsafe.Pointer(&res[0]), unsafe.Pointer(&dat[0]), C.int(len(dat)))
	dat = r.s.Bytes()
	C.bytes2bn(unsafe.Pointer(&res[24]), unsafe.Pointer(&dat[0]), C.int(len(dat)))
	return unsafe.Pointer(&res[0])
}
