package main

/*
  This is a EC_Verify speedup that works only with Windows

  Use secp256k1.dll from gocoin/tools/sipa_dll
  or build one yourself.

*/

import (
	"fmt"
	"unsafe"
	"syscall"
	"github.com/piotrnar/gocoin/lib/btc"
)

var (
	dll = syscall.NewLazyDLL("secp256k1.dll")
	DLL_EC_Verify = dll.NewProc("EC_Verify")
)


func EC_Verify(pkey, sign, hash []byte) bool {
	r1, _, _ := syscall.Syscall6(DLL_EC_Verify.Addr(), 6,
		uintptr(unsafe.Pointer(&hash[0])), uintptr(32),
		uintptr(unsafe.Pointer(&sign[0])), uintptr(len(sign)),
		uintptr(unsafe.Pointer(&pkey[0])), uintptr(len(pkey)))
	return r1==1
}

func init() {
	fmt.Println("Using secp256k1.dll by sipa for EC_Verify")
	btc.EC_Verify = EC_Verify
}
