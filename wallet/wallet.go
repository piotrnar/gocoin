package main

import (
	"os"
	"fmt"
	"bytes"
	"bufio"
	"strings"
	"encoding/hex"
	"github.com/piotrnar/gocoin/lib/btc"
	"github.com/piotrnar/gocoin/lib/others/sys"
)


type walrec struct {
	priv []byte
	label string
	compr bool
	addr *btc.BtcAddr
}


var (
	type2_secret []byte // used to type-2 wallets
	first_seed [32]byte
	// set in make_wallet():
	keys []walrec
	curFee uint64
)


func load_others() {
	f, e := os.Open(RawKeysFilename)
	if e == nil {
		defer f.Close()
		td := bufio.NewReader(f)
		for {
			li, _, _ := td.ReadLine()
			if li == nil {
				break
			}
			if len(li)==0 {
				continue
			}
			pk := strings.SplitN(strings.Trim(string(li), " "), " ", 2)
			if pk[0][0]=='#' {
				continue // Just a comment-line
			}

			pkb := btc.Decodeb58(pk[0])

			if pkb == nil {
				println("Decodeb58 failed:", pk[0])
				continue
			}

			if len(pkb) < 6 {
				println("Syntax error in the raw keys file:", pk[0])
				continue
			}

			if len(pkb)!=37 && len(pkb)!=38 {
				println(pk[0][:6], "has wrong key", len(pkb))
				println(hex.EncodeToString(pkb))
				continue
			}

			if pkb[0]!=AddrVerSecret() {
				println(pk[0][:6], "has version", pkb[0], "while we expect", AddrVerSecret())
				fmt.Println("You may want to play with -t or -ltc switch")
				continue
			}

			var sh [32]byte
			var compr bool

			if len(pkb)==37 {
				// old/uncompressed key
				sh = btc.Sha2Sum(pkb[0:33])
				if !bytes.Equal(sh[:4], pkb[33:37]) {
					println(pk[0][:6], "checksum error")
					continue
				}
				compr = false
			} else {
				if pkb[33]!=1 {
					println(pk[0][:6], "a key of length 38 bytes must be compressed")
					continue
				}

				sh = btc.Sha2Sum(pkb[0:34])
				if !bytes.Equal(sh[:4], pkb[34:38]) {
					println(pk[0][:6], "checksum error")
					continue
				}
				compr = true
			}

			key := pkb[1:33]
			pub := btc.PublicFromPrivate(key, compr)
			if pub == nil {
				println("PublicFromPrivate failed")
				os.Exit(1)
			}

			var rec walrec
			rec.priv = key
			rec.compr = compr
			if len(pk)>1 {
				rec.label = pk[1]
			} else {
				rec.label = fmt.Sprint("Other ", len(keys))
			}
			rec.addr = btc.NewAddrFromPubkey(pub, AddrVerPubkey())
			keys = append(keys, rec)
		}
		if *verbose {
			fmt.Println(len(keys), "keys imported from", RawKeysFilename)
		}
	} else {
		if *verbose {
			fmt.Println("You can also have some dumped (b58 encoded) priv keys in file", RawKeysFilename)
		}
	}
}


// Get the secret seed and generate "keycnt" key pairs (both private and public)
func make_wallet() {
	var lab string

	load_others()

	seed_key := make([]byte, 32)
	if !getseed(seed_key) {
		os.Exit(0)
	}

	defer func() {
		sys.ClearBuffer(seed_key)
	}()

	switch waltype {
		case 1:
			lab = "TypA"
			println("WARNING: Wallet Type 1 is obsolete")

		case 2:
			lab = "TypB"
			if type2sec!="" {
				d, e := hex.DecodeString(type2sec)
				if e!=nil {
					println("t2sec error:", e.Error())
					os.Exit(1)
				}
				type2_secret = d
			} else {
				type2_secret = make([]byte, 20)
				btc.RimpHash(seed_key, type2_secret)
			}

		case 3:
			lab = "TypC"

		default:
			println("ERROR: Unsupported wallet type", waltype)
			os.Exit(0)
	}

	if *verbose {
		fmt.Println("Generating", keycnt, "keys, version", AddrVerPubkey(),"...")
	}
	for i:=uint(0); i < keycnt; {
		prv_key := make([]byte, 32)
		if waltype==3 {
			btc.ShaHash(seed_key, prv_key)
			seed_key = append(seed_key, byte(i))
		} else if waltype==2 {
			seed_key = btc.DeriveNextPrivate(seed_key, type2_secret)
			copy(prv_key, seed_key)
		} else {
			btc.ShaHash(seed_key, prv_key)
			copy(seed_key, prv_key)
		}
		if *scankey!="" {
			new_stealth_address(prv_key)
			return
		}

		// for stealth keys
		if i==0 {
			copy(first_seed[:], prv_key)
		}
		pub := btc.PublicFromPrivate(prv_key, !uncompressed)
		if pub == nil {
			println("PublicFromPrivate error 3")
			continue
		}
		adr := btc.NewAddrFromPubkey(pub, AddrVerPubkey())
		if *pubkey!="" && *pubkey==adr.String() {
			fmt.Println("Public address:", adr.String())
			fmt.Println("Public hexdump:", hex.EncodeToString(pub))
			return
		}

		var rec walrec
		rec.priv = prv_key
		rec.compr = !uncompressed
		rec.addr = adr
		rec.label = fmt.Sprint(lab, " ", i+1)
		keys = append(keys, rec)
		i++
	}
	if *verbose {
		fmt.Println("Private keys re-generated")
	}
}


// Print all the public addresses
func dump_addrs() {
	f, _ := os.Create("wallet.txt")

	fmt.Fprintln(f, "# Deterministic Walet Type", waltype)
	if type2_secret!=nil {
		fmt.Fprintln(f, "#", hex.EncodeToString(keys[0].addr.Pubkey))
		fmt.Fprintln(f, "#", hex.EncodeToString(type2_secret))
	}
	for i := range keys {
		if !*noverify {
			if er := btc.VerifyKeyPair(keys[i].priv, keys[i].addr.Pubkey); er!=nil {
				println("Something wrong with key at index", i, " - abort!", er.Error())
				os.Exit(1)
			}
		}
		fmt.Println(keys[i].addr.String(), keys[i].label)
		if f != nil {
			fmt.Fprintln(f, keys[i].addr.String(), keys[i].label)
		}
	}
	if f != nil {
		f.Close()
		fmt.Println("You can find all the addresses in wallet.txt file")
	}
}
