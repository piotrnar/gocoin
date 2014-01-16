// +build !windows

package main

import (
	"fmt"
	"unsafe"
	"syscall"
	"github.com/piotrnar/gocoin/btc"
	"github.com/piotrnar/gocoin/btc/newec"
)

func EC_Verify_native(k, s, h []byte) bool {
	return newec.Verify(k, s, h)
}

func init() {
	fmt.Println("Using NewEC wrapper for EC_Verify")
	btc.EC_Verify = EC_Verify_native
}
