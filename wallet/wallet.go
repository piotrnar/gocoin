package main

import (
	"os"
	"fmt"
	"bufio"
	"bytes"
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
	segwit []*btc.BtcAddr
	curFee uint64
)


// load private keys fo .others file
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
			if rec.Version!=ver_secret() {
				println(pk[0][:6], "has version", rec.Version, "while we expect", ver_secret())
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

	var seed_key []byte
	var hdwal *btc.HDWallet

	defer func() {
		sys.ClearBuffer(seed_key)
		if hdwal!=nil {
			sys.ClearBuffer(hdwal.Key)
			sys.ClearBuffer(hdwal.ChCode)
		}
	}()

	pass := getpass()
	if pass==nil {
		cleanExit(0)
	}

	if waltype>=1 && waltype<=3 {
		seed_key = make([]byte, 32)
		btc.ShaHash(pass, seed_key)
		sys.ClearBuffer(pass)
		lab = fmt.Sprintf("Typ%c", 'A'+waltype-1)
		if waltype==1 {
			println("WARNING: Wallet Type 1 is obsolete")
		} else if waltype==2 {
			if type2sec!="" {
				d, e := hex.DecodeString(type2sec)
				if e!=nil {
					println("t2sec error:", e.Error())
					cleanExit(1)
				}
				type2_secret = d
			} else {
				type2_secret = make([]byte, 20)
				btc.RimpHash(seed_key, type2_secret)
			}
		}
	} else if waltype==4 {
		lab = "TypHD"
		hdwal = btc.MasterKey(pass, testnet)
		sys.ClearBuffer(pass)
	} else {
		sys.ClearBuffer(pass)
		println("ERROR: Unsupported wallet type", waltype)
		cleanExit(1)
	}

	if *verbose {
		fmt.Println("Generating", keycnt, "keys, version", ver_pubkey(),"...")
	}

	first_determ_idx = len(keys)
	for i:=uint(0); i < keycnt; {
		prv_key := make([]byte, 32)
		if waltype==3 {
			btc.ShaHash(seed_key, prv_key)
			seed_key = append(seed_key, byte(i))
		} else if waltype==2 {
			seed_key = btc.DeriveNextPrivate(seed_key, type2_secret)
			copy(prv_key, seed_key)
		} else if waltype==1 {
			btc.ShaHash(seed_key, prv_key)
			copy(seed_key, prv_key)
		} else /*if waltype==4*/ {
			// HD wallet
			_hd := hdwal.Child(uint32(0x80000000|i))
			copy(prv_key, _hd.Key[1:])
			sys.ClearBuffer(_hd.Key)
			sys.ClearBuffer(_hd.ChCode)
		}
		if *scankey!="" {
			new_stealth_address(prv_key)
			return
		}

		rec := btc.NewPrivateAddr(prv_key, ver_secret(), !uncompressed)

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

	// Calculate SegWit addresses
	segwit = make([]*btc.BtcAddr, len(keys))
	for i, pk := range keys {
		if len(pk.Pubkey)!=33 {
			continue
		}
		if *bech32_mode {
			segwit[i] = btc.NewAddrFromPkScript(append([]byte{0,20}, pk.Hash160[:]...), testnet)
		} else {
			h160 := btc.Rimp160AfterSha256(append([]byte{0,20}, pk.Hash160[:]...))
			segwit[i] = btc.NewAddrFromHash160(h160[:], btc.AddrVerScript(testnet))
		}
	}
}


// Print all the public addresses
func dump_addrs() {
	f, _ := os.Create("wallet.txt")

	fmt.Fprintln(f, "# Deterministic Walet Type", waltype)
	if type2_secret!=nil {
		fmt.Fprintln(f, "#", hex.EncodeToString(keys[first_determ_idx].BtcAddr.Pubkey))
		fmt.Fprintln(f, "#", hex.EncodeToString(type2_secret))
	}
	for i := range keys {
		if !*noverify {
			if er := btc.VerifyKeyPair(keys[i].Key, keys[i].BtcAddr.Pubkey); er!=nil {
				println("Something wrong with key at index", i, " - abort!", er.Error())
				cleanExit(1)
			}
		}
		var pubaddr string
		if *segwit_mode {
			if segwit[i]==nil {
				pubaddr = "-=CompressedKey=-"
			} else {
				pubaddr = segwit[i].String()
			}
		} else {
			pubaddr = keys[i].BtcAddr.String()
		}
		fmt.Println(pubaddr, keys[i].BtcAddr.Extra.Label)
		if f != nil {
			fmt.Fprintln(f, pubaddr, keys[i].BtcAddr.Extra.Label)
		}
	}
 	if f != nil {
		f.Close()
		fmt.Println("You can find all the addresses in wallet.txt file")
	}
}


func public_to_key(pubkey []byte) *btc.PrivateAddr {
	for i := range keys {
		if bytes.Equal(pubkey, keys[i].BtcAddr.Pubkey) {
			return keys[i]
		}
	}
	return nil
}


func hash_to_key_idx(h160 []byte) (res int) {
	for i := range keys {
		if bytes.Equal(keys[i].BtcAddr.Hash160[:], h160) {
			return i
		}
		if segwit[i]!=nil && bytes.Equal(segwit[i].Hash160[:], h160) {
			return i
		}
	}
	return -1
}


func hash_to_key(h160 []byte) *btc.PrivateAddr {
	if i:=hash_to_key_idx(h160); i>=0 {
		return keys[i]
	}
	return nil
}


func address_to_key(addr string) *btc.PrivateAddr {
	a, e := btc.NewAddrFromString(addr)
	if e != nil {
		println("Cannot Decode address", addr)
		println(e.Error())
		cleanExit(1)
	}
	return hash_to_key(a.Hash160[:])
}


// suuports only P2KH scripts
func pkscr_to_key(scr []byte) *btc.PrivateAddr {
	if len(scr)==25 && scr[0]==0x76 && scr[1]==0xa9 && scr[2]==0x14 && scr[23]==0x88 && scr[24]==0xac {
		return hash_to_key(scr[3:23])
	}
	// P2SH(WPKH)
	if len(scr)==23 && scr[0]==0xa9 && scr[22]==0x87 {
		return hash_to_key(scr[2:22])
	}
	return nil
}


func dump_prvkey() {
	if *dumppriv=="*" {
		// Dump all private keys
		for i := range keys {
			fmt.Println(keys[i].String(), keys[i].BtcAddr.String(), keys[i].BtcAddr.Extra.Label)
		}
	} else {
		// single key
		k := address_to_key(*dumppriv)
		if k != nil {
			fmt.Println("Public address:", k.BtcAddr.String(), k.BtcAddr.Extra.Label)
			fmt.Println("Public hexdump:", hex.EncodeToString(k.BtcAddr.Pubkey))
			fmt.Println("Public compressed:", k.BtcAddr.IsCompressed())
			fmt.Println("Private encoded:", k.String())
			fmt.Println("Private hexdump:", hex.EncodeToString(k.Key))
		} else {
			println("Dump Private Key:", *dumppriv, "not found it the wallet")
		}
	}
}
