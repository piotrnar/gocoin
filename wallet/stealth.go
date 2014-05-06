package main

import (
	"os"
	"fmt"
	"flag"
	"crypto/rand"
	"encoding/hex"
	"crypto/sha256"
	"encoding/binary"
	"github.com/piotrnar/gocoin/btc"
	"github.com/piotrnar/gocoin/others/utils"
)

var (
	scankey *string = flag.String("scankey", "", "Generate a new stealth using this public scan-key")
	prefix *uint = flag.Uint("prefix", 0, "Stealth prefix length in bits (maximum 24)")
	is_stealth map[int] bool = make(map[int]bool)
)

// Thanks @dabura667 - https://bitcointalk.org/index.php?topic=590349.msg6560332#msg6560332
func stealth_txout(sa *btc.StealthAddr, value uint64) (res []*btc.TxOut) {
	if sa.Version != btc.StealthAddressVersion(*testnet) {
		fmt.Println("ERROR: Unsupported version of a stealth address", sa.Version)
		os.Exit(1)
	}

	if len(sa.SpendKeys) != 1 {
		fmt.Println("ERROR: Currently only non-multisig stealth addresses are supported",
			len(sa.SpendKeys))
		os.Exit(1)
	}

	// Make two outpus
	res = make([]*btc.TxOut, 2)
	var e, ephemkey, pkscr []byte
	var nonce uint32
	var look4pref bool
	sha := sha256.New()

	// 6. create a new pub/priv keypair (lets call its pubkey "ephemkey" and privkey "e")
pick_different_e:
	e = make([]byte, 32)
	rand.Read(e)
	defer utils.ClearBuffer(e)
	ephemkey = btc.PublicFromPrivate(e, true)
	if *verbose {
		fmt.Println("e", hex.EncodeToString(e))
		fmt.Println("ephemkey", hex.EncodeToString(ephemkey))
	}

	// 7. IF there is a prefix in the stealth address, brute force a nonce such
	// that SHA256(nonce.concate(ephemkey)) first 4 bytes are equal to the prefix.
	// IF NOT, then just run through the loop once and pickup a random nonce.
	// (probably make the while condition include "or prefix = null" or something to that nature.
	look4pref = len(sa.Prefix)>0 && sa.Prefix[0]>0
	if look4pref {
		fmt.Print("Prefix is ", sa.Prefix[0], ":", hex.EncodeToString(sa.Prefix[1:]), " - looking for nonce")
	}
	nonce = 0
	for {
		binary.Write(sha, binary.LittleEndian, nonce)
		sha.Write(ephemkey)

		if sa.CheckPrefix(sha.Sum(nil)[:4]) {
			break
		}
		sha.Reset()

		if nonce==0xffffffff {
			goto pick_different_e
		}
		nonce++
		if (nonce&0xfffff)==0 {
			fmt.Print(".")
		}
	}
	if look4pref {
		fmt.Println()
		fmt.Println("Found prefix", hex.EncodeToString(sha.Sum(nil)[:4]))
	}

	// 8. Once you have the nonce and the ephemkey, you can create the first output, which is
	pkscr = make([]byte, 40)
	pkscr[0] = 0x6a // OP_RETURN
	pkscr[1] = 38 // length
	pkscr[2] = 0x06 // always 6
	binary.LittleEndian.PutUint32(pkscr[3:7], nonce)
	copy(pkscr[7:40], ephemkey)
	res[0] = &btc.TxOut{Pk_script: pkscr}

	// 9. Now use ECC multiplication to calculate e*Q where Q = scan_pubkey
	// an e = privkey to ephemkey and then hash it.
	c := btc.StealthDH(sa.ScanKey[:], e)
	if *verbose {
		fmt.Println("c", hex.EncodeToString(c))
	}

	// 10. That hash is now "c". use ECC multiplication and addition to
	// calculate D + (c*G) where D = spend_pubkey, and G is the reference
	// point for secp256k1. This will give you a new pubkey. (we'll call it D')
	Dpr := btc.DeriveNextPublic(sa.SpendKeys[0][:], c)
	if *verbose {
		fmt.Println("Dpr", hex.EncodeToString(Dpr))
	}

	// 11. Create a normal P2KH output spending to D' as public key.
	adr := btc.NewAddrFromPubkey(Dpr, btc.AddrVerPubkey(*testnet))
	res[1] = &btc.TxOut{Value: value, Pk_script: adr.OutScript() }
	fmt.Println("Sending to stealth", adr.String())

	return
}


// Generate a new stealth address
func new_stealth_address(prv_key []byte) {
	sk, er := hex.DecodeString(*scankey)
	if er != nil {
		println(er.Error())
		os.Exit(1)
	}
	if len(sk)!=33 || sk[0]!=2 && sk[0]!=3 {
		println("scankey must be a compressed public key (33 bytes long)")
		os.Exit(1)
	}

	if *prefix>16 {
		if *prefix>24 {
			fmt.Println("The stealth prefix cannot be bigger than 32", *prefix)
			os.Exit(1)
		}
		fmt.Println("WARNING: You chose a prifix length of", *prefix)
		fmt.Println("WARNING: Big prefixes endanger your anonymity.")
	}

	pub := btc.PublicFromPrivate(prv_key, true)
	if pub == nil {
		println("PublicFromPrivate error 2")
		os.Exit(1)
	}

	sa := new(btc.StealthAddr)
	sa.Version = btc.StealthAddressVersion(*testnet)
	sa.Options = 0
	copy(sa.ScanKey[:], sk)
	sa.SpendKeys = make([][33]byte, 1)
	copy(sa.SpendKeys[0][:], pub)
	sa.Sigs = 1
	sa.Prefix = make([]byte, 1+(byte(*prefix)+7)>>3)
	if *prefix > 0 {
		sa.Prefix[0] = byte(*prefix)
		rand.Read(sa.Prefix[1:])
	}
	fmt.Println(sa.String())
}
