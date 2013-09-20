package main

/*
  This is a EC_Verify speedup that is advised for Windows.

  1) Build secp256k1.dll for your arch (32/64 bit)

  2) Place the secp256k1.dll somewhere within the PATH (i.e. in C:\WINDOWS)

  3) Copy this file one level up and remove "speedup.go" from there

  4) Rebuild clinet.exe and enjoy sipa's verify lib.
*/

import (
	"fmt"
	"unsafe"
	"syscall"
	"github.com/piotrnar/gocoin/btc"
)

var (
	secp256k1 = syscall.NewLazyDLL("secp256k1.dll")
	DLL_EC_Verify = secp256k1.NewProc("EC_Verify")
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
