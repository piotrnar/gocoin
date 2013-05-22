package main

/*
  If you cannot compile the client and you dont know what to do,
  just delete this file adn then "go build" should work, though
  the elliptic curve calculations will be slooooow.
*/

import (
	"github.com/piotrnar/gocoin/openssl"
	"github.com/piotrnar/gocoin/btc"
)

func EC_Verify(k, s, h []byte) bool {
	return openssl.EC_Verify(k, s, h)==1
}

func init() {
	btc.EC_Verify = EC_Verify
}
