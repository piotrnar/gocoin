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

func Schnorr_Verify(pkey, sign, msg []byte) bool {
	r1, _, _ := syscall.Syscall(DLL_Schnorr_Verify.Addr(), 3,
		uintptr(unsafe.Pointer(&msg[0])),
		uintptr(unsafe.Pointer(&sign[0])),
		uintptr(unsafe.Pointer(&pkey[0])))
	return r1 == 1
}

func CheckPayToContract(kd, base, hash []byte, parity bool) bool {
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

func ec_verify() bool {
	key, _ := hex.DecodeString("020eaebcd1df2df853d66ce0e1b0fda07f67d1cabefde98514aad795b86a6ea66d")
	sig, _ := hex.DecodeString("3045022100fe00e013c244062847045ae7eb73b03fca583e9aa5dbd030a8fd1c6dfcf11b1002207d0d04fed8fa1e93007468d5a9e134b0a7023b6d31db4e50942d43a250f4d07c01")
	has, _ := hex.DecodeString("3382219555ddbb5b00e0090f469e590ba1eae03c7f28ab937de330aa60294ed6")
	return EC_Verify(key, sig, has)
}

func schnorr_verify() bool {
	key, _ := hex.DecodeString("DFF1D77F2A671C5F36183726DB2341BE58FEAE1DA2DECED843240F7B502BA659")
	sig, _ := hex.DecodeString("6896BD60EEAE296DB48A229FF71DFE071BDE413E6D43F917DC8DCF8C78DE33418906D11AC976ABCCB20B091292BFF4EA897EFCB639EA871CFA95F6DE339E4B0A")
	msg, _ := hex.DecodeString("243F6A8885A308D313198A2E03707344A4093822299F31D0082EFA98EC4E6C89")
	return Schnorr_Verify(key, sig, msg)
}

func p2scr_verify() bool {
	kd, _ := hex.DecodeString("afaf8a67be00186668f74740e34ffce748139c2b73c9fbd2c1f33e48a612a75d")
	base, _ := hex.DecodeString("f1cbd3f2430910916144d5d2bf63d48a6281e5b8e6ade31413adccff3d8839d4")
	hash, _ := hex.DecodeString("93a760e87123883022cbd462ac40571176cf09d9d2c6168759fee6c2b079fdd8")
	return CheckPayToContract(kd, base, hash, true)
}

func init() {
	if !ec_verify() {
		println("ERROR: Could not initiate secp256k1.dll (EC_Verify failed)")
		os.Exit(1)
	}
	if !schnorr_verify() {
		println("ERROR: Could not initiate secp256k1.dll (Schnorr_Verify failed)")
		os.Exit(1)
	}
	if !p2scr_verify() {
		println("ERROR: Could not initiate secp256k1.dll (CheckPayToContract failed)")
		os.Exit(1)
	}

	common.Log.Println("Using secp256k1.dll of Bitcoin Core for EC_Verify, SchnorrVerify & CheckPayToContact")
	btc.EC_Verify = EC_Verify
	btc.Schnorr_Verify = Schnorr_Verify
	btc.Check_PayToContract = CheckPayToContract
}
