package main

/*
  This is a EC_Verify speedup that works only with Windows

  Use secp256k1.dll from gocoin/tools/sipa_dll
  or build one yourself.

*/

import (
	"encoding/hex"
	"github.com/piotrnar/gocoin/client/common"
	"github.com/piotrnar/gocoin/lib/btc"
	"os"
	"syscall"
	"unsafe"
)

var (
	dll           = syscall.NewLazyDLL("secp256k1.dll")
	DLL_EC_Verify = dll.NewProc("EC_Verify")
	DLL_Schnorr_Verify = dll.NewProc("Schnorr_Verify")
	DLL_CheckPayToContract = dll.NewProc("CheckPayToContract")
)

func EC_Verify(pkey, sign, hash []byte) bool {
	r1, _, _ := syscall.Syscall6(DLL_EC_Verify.Addr(), 6,
		uintptr(unsafe.Pointer(&hash[0])), uintptr(32),
		uintptr(unsafe.Pointer(&sign[0])), uintptr(len(sign)),
		uintptr(unsafe.Pointer(&pkey[0])), uintptr(len(pkey)))
	return r1 == 1
}

func schnorr_ec_verify(pkey, sign, msg []byte) bool {
	r1, _, _ := syscall.Syscall(DLL_Schnorr_Verify.Addr(), 3,
		uintptr(unsafe.Pointer(&msg[0])),
		uintptr(unsafe.Pointer(&sign[0])),
		uintptr(unsafe.Pointer(&pkey[0])))
	return r1 == 1
}

func check_pay_to_contract(kd, base, hash []byte, parity bool) bool {
	var par uintptr
	if parity {
		par = 1
	}
	r1, _, _ := syscall.Syscall6(DLL_CheckPayToContract.Addr(), 4,
		uintptr(unsafe.Pointer(&kd[0])),
		uintptr(unsafe.Pointer(&base[0])),
		uintptr(unsafe.Pointer(&hash[0])), par, 0, 0)
	return r1 == 1
}

func verify() bool {
	key, _ := hex.DecodeString("020eaebcd1df2df853d66ce0e1b0fda07f67d1cabefde98514aad795b86a6ea66d")
	sig, _ := hex.DecodeString("3045022100fe00e013c244062847045ae7eb73b03fca583e9aa5dbd030a8fd1c6dfcf11b1002207d0d04fed8fa1e93007468d5a9e134b0a7023b6d31db4e50942d43a250f4d07c01")
	has, _ := hex.DecodeString("3382219555ddbb5b00e0090f469e590ba1eae03c7f28ab937de330aa60294ed6")
	return EC_Verify(key, sig, has)
}

func init() {
	if verify() {
		common.Log.Println("Using secp256k1.dll of Bitcoin Core for EC_Verify")
		btc.EC_Verify = EC_Verify
	} else {
		common.Log.Println("ERROR: Could not initiate secp256k1.dll")
		os.Exit(1)
	}
}
