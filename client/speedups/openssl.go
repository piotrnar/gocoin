package main

/*
  If you prefer to use OpenSSL implementation for verifying
  transaction signatures:
   1) Copy this file one level up (to the "./client" folder)
   2) Remove "speedup.go" from the client folder
   3) Redo "go build"
*/

import (
	"github.com/piotrnar/gocoin/client/common"
	"github.com/piotrnar/gocoin/lib/btc"
	"github.com/piotrnar/gocoin/lib/others/cgo/openssl"
)

func EC_Verify(k, s, h []byte) bool {
	if len(s) >= 9 && int(s[1]) == len(s)-3 {
		// remove hashType from the signature as new openssl libs do not like it
		// see: https://github.com/piotrnar/gocoin/issues/19
		s = s[:len(s)-1]
	}
	return openssl.EC_Verify(k, s, h) == 1
}

func init() {
	common.Log.Println("Using OpenSSL wrapper for EC_Verify")
	btc.EC_Verify = EC_Verify
}
