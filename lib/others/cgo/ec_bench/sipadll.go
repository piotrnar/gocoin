package main

import (
	"encoding/hex"
	"runtime"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

var (
	secp256k1     = syscall.NewLazyDLL("secp256k1.dll")
	DLL_EC_Verify = secp256k1.NewProc("EC_Verify")
)

func EC_Verify(pkey, sign, hash []byte) int32 {
	r1, _, _ := syscall.Syscall6(DLL_EC_Verify.Addr(), 6,
		uintptr(unsafe.Pointer(&hash[0])), uintptr(32),
		uintptr(unsafe.Pointer(&sign[0])), uintptr(len(sign)),
		uintptr(unsafe.Pointer(&pkey[0])), uintptr(len(pkey)))
	return int32(r1)
}

var CNT int = 100e3

func main() {
	key, _ := hex.DecodeString("040eaebcd1df2df853d66ce0e1b0fda07f67d1cabefde98514aad795b86a6ea66dbeb26b67d7a00e2447baeccc8a4cef7cd3cad67376ac1c5785aeebb4f6441c16")
	sig, _ := hex.DecodeString("3045022100fe00e013c244062847045ae7eb73b03fca583e9aa5dbd030a8fd1c6dfcf11b1002207d0d04fed8fa1e93007468d5a9e134b0a7023b6d31db4e50942d43a250f4d07c01")
	msg, _ := hex.DecodeString("3382219555ddbb5b00e0090f469e590ba1eae03c7f28ab937de330aa60294ed6")
	var wg sync.WaitGroup
	max_routines := make(chan bool, 2*runtime.NumCPU())
	println("Number of threads:", cap(max_routines))
	sta := time.Now()
	for i := 0; i < CNT; i++ {
		wg.Add(1)
		max_routines <- true
		go func() {
			if EC_Verify(key, sig, msg) != 1 {
				println("Verify error")
			}
			wg.Done()
			<-max_routines
		}()
	}
	wg.Wait()
	tim := time.Since(sta)
	println((tim/time.Duration(CNT)).String(), "per ECDSA_Verify")
}
