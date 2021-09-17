package spiadll

import (
	"unsafe"
	"syscall"
)

var (
	dll = syscall.NewLazyDLL("secp256k1.dll")
	DLL_EC_Verify = dll.NewProc("EC_Verify")
	DLL_Schnorr_Verify = dll.NewProc("Schnorr_Verify")
	DLL_CheckPayToContract = dll.NewProc("CheckPayToContract")
)


func EC_Verify(pkey, sign, hash []byte) int32 {
	r1, _, _ := syscall.Syscall6(DLL_EC_Verify.Addr(), 6,
		uintptr(unsafe.Pointer(&hash[0])), uintptr(32),
		uintptr(unsafe.Pointer(&sign[0])), uintptr(len(sign)),
		uintptr(unsafe.Pointer(&pkey[0])), uintptr(len(pkey)))
	return int32(r1)
}


func Schnorr_Verify(pkey, sign, msg []byte) int {
	r1, _, _ := syscall.Syscall(DLL_Schnorr_Verify.Addr(), 3,
		uintptr(unsafe.Pointer(&msg[0])),
		uintptr(unsafe.Pointer(&sign[0])),
		uintptr(unsafe.Pointer(&pkey[0])))
	return int(r1)
}

func CheckPayToContract(kd, base, hash []byte, parity int) int {
	r1, _, _ := syscall.Syscall6(DLL_CheckPayToContract.Addr(), 4,
		uintptr(unsafe.Pointer(&kd[0])),
		uintptr(unsafe.Pointer(&base[0])),
		uintptr(unsafe.Pointer(&hash[0])), uintptr(parity), 0, 0)
	return int(r1)
}
