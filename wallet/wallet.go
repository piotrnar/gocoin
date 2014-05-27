package main

import (
	"os"
	"fmt"
	"bufio"
	"strings"
	"encoding/hex"
	"github.com/piotrnar/gocoin/lib/btc"
	"github.com/piotrnar/gocoin/lib/others/sys"
)


var (
	type2_secret []byte // used to type-2 wallets
	first_determ_idx int
	// set in make_wallet():
	keys []*btc.PrivateAddr
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

			rec, er := btc.DecodePrivateAddr(pk[0])
			if er != nil {
				println("DecodePrivateAddr error:", er.Error())
				if *verbose {
					println(pk[0])
				}
				continue
			}
			if rec.Version!=AddrVerSecret() {
				println(pk[0][:6], "has version", rec.Version, "while we expect", AddrVerSecret())
				fmt.Println("You may want to play with -t or -ltc switch")
			}
			if len(pk)>1 {
				rec.BtcAddr.Extra.Label = pk[1]
			} else {
				rec.BtcAddr.Extra.Label = fmt.Sprint("Other ", len(keys))
			}
			keys = append(keys, rec)
		}
		if *verbose {
			fmt.Println(len(keys), "keys imported from", RawKeysFilename)
		}
	} else {
		if *verbose {
			fmt.Println("You can also have some dumped (b58 encoded) Key keys in file", RawKeysFilename)
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
			first_determ_idx = len(keys)
		}

		rec := btc.NewPrivateAddr(prv_key, AddrVerSecret(), !uncompressed)

		if *pubkey!="" && *pubkey==rec.BtcAddr.String() {
			fmt.Println("Public address:", rec.BtcAddr.String())
			fmt.Println("Public hexdump:", hex.EncodeToString(rec.BtcAddr.Pubkey))
			return
		}

		rec.BtcAddr.Extra.Label = fmt.Sprint(lab, " ", i+1)
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
		fmt.Fprintln(f, "#", hex.EncodeToString(keys[0].BtcAddr.Pubkey))
		fmt.Fprintln(f, "#", hex.EncodeToString(type2_secret))
	}
	for i := range keys {
		if !*noverify {
			if er := btc.VerifyKeyPair(keys[i].Key, keys[i].BtcAddr.Pubkey); er!=nil {
				println("Something wrong with key at index", i, " - abort!", er.Error())
				os.Exit(1)
			}
		}
		fmt.Println(keys[i].BtcAddr.String(), keys[i].BtcAddr.Extra.Label)
		if f != nil {
			fmt.Fprintln(f, keys[i].BtcAddr.String(), keys[i].BtcAddr.Extra.Label)
		}
	}
	if f != nil {
		f.Close()
		fmt.Println("You can find all the addresses in wallet.txt file")
	}
}
