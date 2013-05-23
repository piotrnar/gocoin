package main

/*
  If you want EC verify operations to work about 10 times faster,
  you can try using OpenSSL cgo wrapper, from the "openssl" dir.

  In order to do it, just copy this file one level up (to the
  "client" folder) and try "go build" there.

  If it complains about not being able to build
  "github.com/piotrnar/gocoin/openssl", either try to fix it,
  or just remove the file you copied and continue in slow mode.

  It works on Linux, but so far not seen it working in Windows.
*/

import (
	"github.com/piotrnar/gocoin/openssl"
	"github.com/piotrnar/gocoin/btc"
)

func EC_Verify(k, s, h []byte) bool {
	return openssl.EC_Verify(k, s, h)==1
}

func init() {
	fmt.Println("Using OpenSSL wrapper for libcrypto.a")
	btc.EC_Verify = EC_Verify
}
