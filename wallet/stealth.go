package main

import (
	"os"
	"fmt"
	"crypto/rand"
	"encoding/hex"
	"github.com/piotrnar/gocoin/btc"
	"github.com/piotrnar/gocoin/others/utils"
)


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

	// 6. create a new pub/priv keypair (lets call its pubkey "ephemkey" and privkey "e")
	e := make([]byte, 32)
	rand.Read(e)
	defer utils.ClearBuffer(e)
	ephemkey, _ := btc.PublicFromPrivate(e, true)
	fmt.Println("e", hex.EncodeToString(e))
	fmt.Println("ephemkey", hex.EncodeToString(ephemkey))

	// 7. IF there is a prefix in the stealth address, brute force a nonce such
	// that SHA256(nonce.concate(ephemkey)) first 4 bytes are equal to the prefix.
	// IF NOT, then just run through the loop once and pickup a random nonce.
	// (probably make the while condition include "or prefix = null" or something to that nature.
	// TODO
	var prefix [4]byte
	rand.Read(prefix[:])

	// 8. Once you have the nonce and the ephemkey, you can create the first output, which is
	res[0] = &btc.TxOut{Pk_script: append([]byte{0x6a,0x26,0x06}, append(prefix[:], ephemkey...)...)}

	// 9. Now use ECC multiplication to calculate e*Q where Q = scan_pubkey
	// an e = privkey to ephemkey and then hash it.
	c := btc.StealthDH(sa.ScanKey[:], e)

	// 10. That hash is now "c". use ECC multiplication and addition to
	// calculate D + (c*G) where D = spend_pubkey, and G is the reference
	// point for secp256k1. This will give you a new pubkey. (we'll call it D')
	Dpr := btc.DeriveNextPublic(sa.SpendKeys[0][:], c)
	fmt.Println("Dpr", hex.EncodeToString(Dpr))

	// 11. Create a normal P2KH output spending to D' as public key.
	adr := btc.NewAddrFromPubkey(Dpr, btc.AddrVerPubkey(*testnet))
	res[1] = &btc.TxOut{Value: value, Pk_script: adr.OutScript() }
	fmt.Println("Sending to stealth", adr.String())

	return
}
