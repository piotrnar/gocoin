package gocoin
// This file is only to make "go get" working

import (
	_ "github.com/dchest/siphash"
	_ "github.com/golang/snappy"
	_ "golang.org/x/crypto/ripemd160"
)
const Version = "1.9.1"
