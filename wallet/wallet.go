package main

import (
	"os"
	"fmt"
	"bytes"
	"bufio"
	"strings"
	"encoding/hex"
	"github.com/piotrnar/gocoin/btc"
)


// get public key in bitcoin protocol format, from the give private key
func priv2pub(curv *btc.BitCurve, priv_key []byte, compressed bool) (res []byte) {
	x, y := curv.ScalarBaseMult(priv_key)
	xd := x.Bytes()

	if len(xd)>32 {
		println("x is too long:", len(xd))
		os.Exit(2)
	}

	if !compressed {
		yd := y.Bytes()
		if len(yd)>32 {
			println("y is too long:", len(yd))
			os.Exit(2)
		}

		res = make([]byte, 65)
		res[0] = 4
		copy(res[1+32-len(xd):33], xd)
		copy(res[33+32-len(yd):65], yd)
	} else {
		res = make([]byte, 33)
		res[0] = 2+byte(y.Bit(0)) // 02 for even Y values, 03 for odd..
		copy(res[1+32-len(xd):33], xd)
	}

	return
}

func load_others() {
	f, e := os.Open("others.sec")
	if e == nil {
		defer f.Close()
		td := bufio.NewReader(f)
		for {
			li, _, _ := td.ReadLine()
			if li == nil {
				break
			}
			pk := strings.SplitN(strings.Trim(string(li), " "), " ", 2)
			pkb := btc.Decodeb58(pk[0])
			if pkb == nil {
				println("Decodeb58 failed:", pk[0])
				continue
			}

			if len(pkb)!=37 && len(pkb)!=38 {
				println(pk[0], "has wrong key", len(pkb))
				println(hex.EncodeToString(pkb))
				continue
			}

			if pkb[0]!=privver {
				println(pk[0], "has version", pkb[0], "while we expect", privver)
				if pkb[0]==0xef {
					fmt.Println("We guess you meant testnet, so switching to testnet mode...")
					privver = 0xef
					verbyte = 0x6f
				} else {
					continue
				}
			}

			var sh [32]byte
			var compr bool

			if len(pkb)==37 {
				// compressed key
				//println(pk[0], "is compressed")
				sh = btc.Sha2Sum(pkb[0:33])
				if !bytes.Equal(sh[:4], pkb[33:37]) {
					println(pk[0], "checksum error")
					continue
				}
				compr = false
			} else {
				if pkb[33]!=1 {
					println("we only support compressed keys of length 38 bytes", pk[0])
					continue
				}

				sh = btc.Sha2Sum(pkb[0:34])
				if !bytes.Equal(sh[:4], pkb[34:38]) {
					println(pk[0], "checksum error")
					continue
				}
				compr = true
			}

			var key [32]byte
			copy(key[:], pkb[1:33])
			priv_keys = append(priv_keys, key)
			publ_addrs = append(publ_addrs,
				btc.NewAddrFromPubkey(priv2pub(curv, key[:], compr), verbyte))
			if len(pk)>1 {
				labels = append(labels, pk[1])
			} else {
				labels = append(labels, fmt.Sprint("Other ", len(priv_keys)))
			}
		}
		fmt.Println(len(priv_keys), "keys imported")
	} else {
		fmt.Println("You can also have some dumped (b58 encoded) priv keys in 'others.sec'")
	}
}


// Get the secret seed and generate "*keycnt" key pairs (both private and public)
func make_wallet() {
	if *testnet {
		verbyte = 0x6f
		privver = 0xef
	} else {
		// verbyte is be zero by definition
		privver = 0x80
	}
	load_others()

	pass := getpass()
	seed_key := btc.Sha2Sum([]byte(pass))
	if pass!="" {
		fmt.Println("Generating", *keycnt, "keys, version", verbyte,"...")
		for i:=uint(0); i < *keycnt; i++ {
			seed_key = btc.Sha2Sum(seed_key[:])
			priv_keys = append(priv_keys, seed_key)
			publ_addrs = append(publ_addrs,
				btc.NewAddrFromPubkey(priv2pub(curv, seed_key[:], !*uncompressed), verbyte))
			labels = append(labels, fmt.Sprint("Auto ", i+1))
		}
		fmt.Println("Private keys re-generated")
	}
}
