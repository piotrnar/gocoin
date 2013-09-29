package spiadll

import (
	"unsafe"
	"syscall"
)

var (
	advapi32 = syscall.NewLazyDLL("secp256k1.dll")
	DLL_EC_Verify = advapi32.NewProc("EC_Verify")
)


func EC_Verify(pkey, sign, hash []byte) int32 {
	r1, _, _ := syscall.Syscall6(DLL_EC_Verify.Addr(), 6,
		uintptr(unsafe.Pointer(&hash[0])), uintptr(32),
		uintptr(unsafe.Pointer(&sign[0])), uintptr(len(sign)),
		uintptr(unsafe.Pointer(&pkey[0])), uintptr(len(pkey)))
	return int32(r1)
}
