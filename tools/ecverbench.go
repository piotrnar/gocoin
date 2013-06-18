package main

import (
	"os"
	"time"
	"encoding/hex"
	"github.com/piotrnar/gocoin/btc"
	"github.com/piotrnar/gocoin/cgo/openssl"
	"github.com/piotrnar/gocoin/cgo/sipasec"
)

func O_Verify(k, s, h []byte) bool {
	return openssl.EC_Verify(k, s, h)==1
}

func S_Verify(k, s, h []byte) bool {
	return sipasec.EC_Verify(k, s, h)==1
}


var CNT int = 500
const THREADS = 8

func main() {
	if len(os.Args)>1 {
		switch os.Args[1] {
			case "openssl":
				btc.EC_Verify = O_Verify
				CNT *= 10
			case "sipasec":
				btc.EC_Verify = S_Verify
				CNT *= 50
			default:
				println("The only allowed parameters are either 'openssl' or 'sipasec'")
				return
		}
	}

	key, _ := hex.DecodeString("040eaebcd1df2df853d66ce0e1b0fda07f67d1cabefde98514aad795b86a6ea66dbeb26b67d7a00e2447baeccc8a4cef7cd3cad67376ac1c5785aeebb4f6441c16")
	sig, _ := hex.DecodeString("3045022100fe00e013c244062847045ae7eb73b03fca583e9aa5dbd030a8fd1c6dfcf11b1002207d0d04fed8fa1e93007468d5a9e134b0a7023b6d31db4e50942d43a250f4d07c01")
	msg, _ := hex.DecodeString("3382219555ddbb5b00e0090f469e590ba1eae03c7f28ab937de330aa60294ed6")
	sta := time.Now()
	ch := make(chan bool, THREADS)
	for i:=0; i<THREADS; i++ {
		ch <- true
	}
	var ok bool
	for i:=0; i<CNT; i++ {
		ok = <- ch
		if !ok {
			println("Verify error")
			return
		}
		go func(k, s, h []byte) {
			ch <- btc.EcdsaVerify(k, s, h)
		}(key, sig, msg)
	}
	for i:=0; i<THREADS; i++ {
		ok = <- ch
		if !ok {
			println("Verify error")
			return
		}
	}
	if len(ch)!=0 {
		panic("channel not empty")
	}
	sto := time.Now()
	println((sto.UnixNano()-sta.UnixNano())/int64(CNT*1000), "us per ECDSA_Verify")
}
