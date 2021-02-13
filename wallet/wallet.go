package main

import (
	"os"
	"fmt"
	"bufio"
	"bytes"
	"strings"
	"encoding/hex"
    "crypto/sha256"
	"github.com/piotrnar/gocoin/lib/btc"
	"github.com/piotrnar/gocoin/lib/others/sys"
    "github.com/piotrnar/gocoin/lib/others/bip39"
)


var (
	first_determ_idx int
	// set in make_wallet():
	keys []*btc.PrivateAddr
	segwit []*btc.BtcAddr
	curFee uint64
    hd_wallet_path string
    hd_wallet_xpub string
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


// make_wallet gets the secret seed and generates "keycnt" key pairs (both private and public).
func make_wallet() {
	var lab string
    var hd_hard uint32

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

    if waltype < 1 || waltype > 4 {
		println("ERROR: Unsupported wallet type", waltype)
		os.Exit(1)
    }
    
    if waltype < 3 {
		println("Wallets Type", waltype, " are no longer supported. Use Gocoin wallet 1.9.8 or earlier.")
		os.Exit(1)
    }
    
    if waltype == 4 {
        if hdwaltype > 4 {
            println("ERROR: Incorrect value of HD Wallet type", hdwaltype)
            os.Exit(1)
        }
    }
    
    if bip39bits != 0 {
        if bip39bits < 128 || bip39bits > 256 || (bip39bits % 32) != 0 {
            println("ERROR: Incorrect value for BIP39 entropy bits", bip39bits)
            os.Exit(1)
        }
        if waltype != 4 {
            fmt.Println("WARNING: Not HD-Wallet type. BIP39 mode ignored.")
        }
    }

    pass := getpass()
	if pass==nil {
		cleanExit(0)
	}

	if waltype == 3 {
		seed_key = make([]byte, 32)
		btc.ShaHash(pass, seed_key)
		sys.ClearBuffer(pass)
		lab = "TypC"
	} else /*if waltype==4*/ {
        lab = fmt.Sprint("TypHD", hdwaltype)
        if bip39bits != 0 {
            lab = fmt.Sprint(lab, "-b", bip39bits)
            s := sha256.New()
            s.Write(pass)
            s.Write([]byte("|gocoin|"))
            s.Write(pass)
            s.Write([]byte{byte(bip39bits)})
            seed_key = s.Sum(nil)
            sys.ClearBuffer(pass)
            mnemonic, er := bip39.NewMnemonic(seed_key[:bip39bits/8])
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
        if *dumpxprv {
            fmt.Println(hdwal.String())
        }
        if hdwaltype == 0 {
            hd_hard = 0x80000000
            hd_wallet_path = "m/k'"
        } else if hdwaltype == 1 {
            hd_wallet_path = "m/0/k"
            hdwal = hdwal.Child(0)
        } else if hdwaltype == 2 {
            hd_wallet_path = "m/0'/0'/k'"
            hdwal = hdwal.Child(0x80000000).Child(0x80000000)
            hd_hard = 0x80000000
        } else if hdwaltype == 3 {
            hd_wallet_path = "m/0'/0/k"
            hdwal = hdwal.Child(0x80000000).Child(0)
        } else if hdwaltype == 4 {
            hd_wallet_path = "m/44'/0'/0'/k"
            hdwal = hdwal.Child(0x80000000+44).Child(0x80000000).Child(0x80000000)
        } /*else if hdwaltype == 5 {   // for importing word-list into electrum
            hd_wallet_path = "m/44'/0'/0'/0/k"
            hdwal = hdwal.Child(0x80000000+44).Child(0x80000000).Child(0x80000000).Child(0)
        }*/
        if *dumpxprv {
            fmt.Println(hdwal.String())
        }
        if hd_hard == 0 {
            hd_wallet_xpub = hdwal.Pub().String()
        }
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
		} else /*if waltype==4*/ {
			// HD wallet
            _hd := hdwal.Child(uint32(i)|hd_hard)
			copy(prv_key, _hd.Key[1:])
			sys.ClearBuffer(_hd.Key)
			sys.ClearBuffer(_hd.ChCode)
		}

		rec := btc.NewPrivateAddr(prv_key, ver_secret(), !uncompressed)

		if *pubkey!="" && *pubkey==rec.BtcAddr.String() {
            sys.ClearBuffer(seed_key)
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
			segwit[i] = btc.NewAddrFromHash160(h160[:], ver_script())
		}
	}
}


// dump_addrs prints all the public addresses.
func dump_addrs() {
	f, _ := os.Create("wallet.txt")

	fmt.Fprintln(f, "# Deterministic Walet Type", waltype)
    if hd_wallet_path != "" {
        fmt.Fprintln(f, "# BIP32 Derivation Path:", hd_wallet_path)
    }
    if hd_wallet_xpub != "" {
        fmt.Fprintln(f, "# Extended Pubkey:", hd_wallet_xpub)
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


// pkscr_to_key supports only P2KH scripts.
func pkscr_to_key(scr []byte) *btc.PrivateAddr {
	if len(scr)==25 && scr[0]==0x76 && scr[1]==0xa9 && scr[2]==0x14 && scr[23]==0x88 && scr[24]==0xac {
		return hash_to_key(scr[3:23])
	}
	// P2SH(WPKH)
	if len(scr)==23 && scr[0]==0xa9 && scr[22]==0x87 {
		return hash_to_key(scr[2:22])
	}
	// P2WPKH
	if len(scr)==22 && scr[0]==0x00 && scr[1]==0x14 {
		return hash_to_key(scr[2:])
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
