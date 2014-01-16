package main

import (
	"fmt"
	"unsafe"
	"syscall"
	"github.com/piotrnar/gocoin/btc"
	"github.com/piotrnar/gocoin/btc/newec"
)

var (
	secp256k1 = syscall.NewLazyDLL("secp256k1.dll")
	DLL_EC_Verify = secp256k1.NewProc("EC_Verify")
)

func EC_Verify_sipa(pkey, sign, hash []byte) bool {
	r1, _, _ := syscall.Syscall6(DLL_EC_Verify.Addr(), 6,
		uintptr(unsafe.Pointer(&hash[0])), uintptr(32),
		uintptr(unsafe.Pointer(&sign[0])), uintptr(len(sign)),
		uintptr(unsafe.Pointer(&pkey[0])), uintptr(len(pkey)))
	return r1==1
}

func EC_Verify_native(k, s, h []byte) bool {
	return newec.Verify(k, s, h)
}

func init() {
	defer func() {
		if r := recover(); r != nil {
			fmt.Println("Using NewEC wrapper for EC_Verify")
			btc.EC_Verify = EC_Verify_native
		}
	}()

	if secp256k1!=nil && DLL_EC_Verify!=nil {
		btc.EC_Verify = EC_Verify_sipa
	}
	if DLL_EC_Verify.Addr()!=0 {
		fmt.Println("Using secp256k1.dll by sipa for EC_Verify")
	}
}
