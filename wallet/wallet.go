package main

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/piotrnar/gocoin/lib/btc"
	"github.com/piotrnar/gocoin/lib/others/bip39"
	"github.com/piotrnar/gocoin/lib/others/sys"
)

var (
	first_determ_idx int
	// set in make_wallet():
	keys           []*btc.PrivateAddr
	segwit         []*btc.BtcAddr
	curFee         uint64
	hd_wallet_xtra []string
)

// load_others loads private keys of .others file.
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
			if len(li) == 0 {
				continue
			}
			pk := strings.SplitN(strings.Trim(string(li), " "), " ", 2)
			if pk[0][0] == '#' {
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
			if rec.Version != ver_secret() {
				println(pk[0][:6], "has version", rec.Version, "while we expect", ver_secret())
				fmt.Println("You may want to play with -t or -ltc switch")
			}
			if len(pk) > 1 {
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

// make_wallet gets the secret seed and generates "keycnt" key pairs (both private and public).
func make_wallet() {
	var seed_key []byte
	var hdwal *btc.HDWallet
	var hdpath_x []uint32
	var hdpath_last uint32
	var hd_label_prefix string

	load_others()
	defer func() {
		sys.ClearBuffer(seed_key)
		if hdwal != nil {
			sys.ClearBuffer(hdwal.Key)
			sys.ClearBuffer(hdwal.ChCode)
		}
	}()

	if waltype < 1 || waltype > 4 {
		println("ERROR: Unsupported wallet type", waltype)
		os.Exit(1)
	}

	if waltype < 3 {
		println("Wallets Type", waltype, " are no longer supported. Use Gocoin wallet 1.9.8 or earlier.")
		os.Exit(1)
	}

	if waltype == 4 {
		// parse hdpath
		ts := strings.Split(hdpath, "/")
		if len(ts) < 2 || ts[0] != "m" {
			println("hdpath - top level syntax error:", hdpath, len(ts))
			os.Exit(1)
		}
		hd_label_prefix = "m"
		for i := 1; i < len(ts); i++ {
			var xval uint32
			ti := ts[i]
			if strings.HasSuffix(ti, "'") {
				xval = 0x80000000 // hardened
			}
			if v, e := strconv.ParseInt(strings.TrimSuffix(ti, "'"), 10, 32); e != nil || v < 0 {
				println("hdpath - syntax error. non-negative integer expected:", ti)
				os.Exit(1)
			} else {
				xval |= uint32(v)
			}
			hdpath_x = append(hdpath_x, xval)
			if i < len(ts)-1 {
				hd_label_prefix += fmt.Sprint("/", xval&0x3fffffff)
				if (xval & 0x80000000) != 0 {
					hd_label_prefix += "'"
				}
			}
		}
		hdpath_last = hdpath_x[len(hdpath_x)-1]
	}

	if bip39wrds != 0 {
		if bip39wrds < 12 || bip39wrds > 24 || (bip39wrds%3) != 0 {
			println("ERROR: Incorrect value for BIP39 words count", bip39wrds)
			os.Exit(1)
		}
		if waltype != 4 {
			fmt.Println("WARNING: Not HD-Wallet type. BIP39 mode ignored.")
		}
	}

	pass := getpass()
	if pass == nil {
		cleanExit(0)
	}

	if waltype == 3 {
		seed_key = make([]byte, 32)
		btc.ShaHash(pass, seed_key)
		sys.ClearBuffer(pass)
	} else /*if waltype==4*/ {
		if bip39wrds != 0 {
			hd_wallet_xtra = append(hd_wallet_xtra, fmt.Sprint("Based on ", bip39wrds, " BIP39 words"))
			s := sha256.New()
			s.Write(pass)
			s.Write([]byte("|gocoin|"))
			s.Write(pass)
			s.Write([]byte{byte(bip39wrds / 3 * 32)})
			seed_key = s.Sum(nil)
			sys.ClearBuffer(pass)
			mnemonic, er := bip39.NewMnemonic(seed_key[:(bip39wrds/3)*4])
			sys.ClearBuffer(seed_key)
			if er != nil {
				println(er.Error())
				cleanExit(1)
			}
			if *dumpwords {
				fmt.Println("BIP39:", mnemonic)
			}
			seed_key, er = bip39.NewSeedWithErrorChecking(mnemonic, "")
			hdwal = btc.MasterKey(seed_key, testnet)
			sys.ClearBuffer(seed_key)
		} else {
			hdwal = btc.MasterKey(pass, testnet)
			sys.ClearBuffer(pass)
		}
		if !testnet && *segwit_mode {
			if *bech32_mode {
				hdwal.Prefix = btc.PrivateZ
			} else {
				hdwal.Prefix = btc.PrivateY
			}
		}
		if *dumpxprv {
			fmt.Println(hdwal.String())
		}
		for _, x := range hdpath_x[:len(hdpath_x)-1] {
			hdwal = hdwal.Child(x)
		}
		if *dumpxprv {
			fmt.Println(hdwal.String())
		}
		if (hdpath_last & 0x80000000) == 0 {
			hd_wallet_xtra = append(hd_wallet_xtra, hdwal.Pub().String())
		}
	}

	if *verbose {
		fmt.Println("Generating", keycnt, "keys, version", ver_pubkey(), "...")
	}

	first_determ_idx = len(keys)
	for i := uint32(0); i < uint32(keycnt); {
		prv_key := make([]byte, 32)
		if waltype == 3 {
			btc.ShaHash(seed_key, prv_key)
			seed_key = append(seed_key, byte(i))
		} else /*if waltype==4*/ {
			// HD wallet
			_hd := hdwal.Child(uint32(i) + hdpath_last)
			copy(prv_key, _hd.Key[1:])
			sys.ClearBuffer(_hd.Key)
			sys.ClearBuffer(_hd.ChCode)
		}

		rec := btc.NewPrivateAddr(prv_key, ver_secret(), !uncompressed)

		if *pubkey != "" && *pubkey == rec.BtcAddr.String() {
			sys.ClearBuffer(seed_key)
			fmt.Println("Public address:", rec.BtcAddr.String())
			fmt.Println("Public hexdump:", hex.EncodeToString(rec.BtcAddr.Pubkey))
			return
		}

		if waltype == 3 {
			rec.BtcAddr.Extra.Label = fmt.Sprint("TypC ", i+1)
		} else {
			if (hdpath_last & 0x80000000) != 0 {
				rec.BtcAddr.Extra.Label = fmt.Sprint(hd_label_prefix, "/", i+(hdpath_last&0x3fffffff), "'")
			} else {
				rec.BtcAddr.Extra.Label = fmt.Sprint(hd_label_prefix, "/", i+(hdpath_last&0x3fffffff))
			}
		}
		keys = append(keys, rec)
		i++
	}
	if *verbose {
		fmt.Println("Private keys re-generated")
	}

	// Calculate SegWit addresses
	segwit = make([]*btc.BtcAddr, len(keys))
	for i, pk := range keys {
		if len(pk.Pubkey) != 33 {
			continue
		}
		if *bech32_mode {
			if *taproot_mode {
				segwit[i] = btc.NewAddrFromPkScript(append([]byte{btc.OP_1, 32}, pk.Pubkey[1:]...), testnet)
			} else {
				segwit[i] = btc.NewAddrFromPkScript(append([]byte{0, 20}, pk.Hash160[:]...), testnet)
			}
		} else {
			h160 := btc.Rimp160AfterSha256(append([]byte{0, 20}, pk.Hash160[:]...))
			segwit[i] = btc.NewAddrFromHash160(h160[:], ver_script())
		}
	}
}

// dump_addrs prints all the public addresses.
func dump_addrs() {
	f, _ := os.Create("wallet.txt")

	fmt.Fprintln(f, "# Deterministic Walet Type", waltype)
	for _, x := range hd_wallet_xtra {
		fmt.Println("#", x)
		fmt.Fprintln(f, "#", x)
	}
	for i := range keys {
		if !*noverify {
			if er := btc.VerifyKeyPair(keys[i].Key, keys[i].BtcAddr.Pubkey); er != nil {
				println("Something wrong with key at index", i, " - abort!", er.Error())
				cleanExit(1)
			}
		}
		var pubaddr string
		if *segwit_mode {
			if segwit[i] == nil {
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

func public_to_key_idx(pubkey []byte) int {
	for i := range keys {
		if bytes.Equal(pubkey, keys[i].BtcAddr.Pubkey) {
			return i
		}
	}
	return -1
}

func public_to_key(pubkey []byte) *btc.PrivateAddr {
	idx := public_to_key_idx(pubkey)
	if idx < 0 {
		return nil
	}
	return keys[idx]
}

func hash_to_key_idx(h160 []byte) (res int) {
	for i := range keys {
		if bytes.Equal(keys[i].BtcAddr.Hash160[:], h160) {
			return i
		}
		if segwit[i] != nil && bytes.Equal(segwit[i].Hash160[:], h160) {
			return i
		}
	}
	return -1
}

func hash_to_key(h160 []byte) *btc.PrivateAddr {
	if i := hash_to_key_idx(h160); i >= 0 {
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

// pkscr_to_key supports only P2KH scripts.
func pkscr_to_key(scr []byte) *btc.PrivateAddr {
	if len(scr) == 25 && scr[0] == 0x76 && scr[1] == 0xa9 && scr[2] == 0x14 && scr[23] == 0x88 && scr[24] == 0xac {
		return hash_to_key(scr[3:23])
	}
	// P2SH(WPKH)
	if len(scr) == 23 && scr[0] == 0xa9 && scr[22] == 0x87 {
		return hash_to_key(scr[2:22])
	}
	// P2WPKH
	if len(scr) == 22 && scr[0] == 0x00 && scr[1] == 0x14 {
		return hash_to_key(scr[2:])
	}
	return nil
}

func dump_prvkey() {
	if *dumppriv == "*" {
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
