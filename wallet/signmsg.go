package main

import (
	"os"
	"fmt"
	"io/ioutil"
	"encoding/hex"
	"encoding/base64"
	"github.com/piotrnar/gocoin/lib/btc"
	"github.com/piotrnar/gocoin/lib/others/ltc"
)


// sign_message signs either a message or a raw hash.
func sign_message() {
	var hash []byte
	var signkey *btc.PrivateAddr

	signkey = address_to_key(*signaddr)
	if signkey==nil {
		println("You do not have a private key for", *signaddr)
		return
	}

	if *signhash!="" {
		hash, er := hex.DecodeString(*signhash)
		if er != nil {
			println("Incorrect content of -hash parameter")
			println(er.Error())
			return
		} else if len(hash)>0 {
			txsig := new(btc.Signature)
			txsig.HashType = 0x01
			r, s, e := btc.EcdsaSign(signkey.Key, hash)
			if e != nil {
				println(e.Error())
				return
			}
			txsig.R.Set(r)
			txsig.S.Set(s)
			fmt.Println("PublicKey:", hex.EncodeToString(signkey.BtcAddr.Pubkey))
			fmt.Println(hex.EncodeToString(txsig.Bytes()))
			return
		}
	}

	var msg []byte
	if *message=="" {
		msg, _ = ioutil.ReadAll(os.Stdin)
	} else {
		msg = []byte(*message)
	}

	hash = make([]byte, 32)
	if litecoin {
		ltc.HashFromMessage(msg, hash)
	} else {
		btc.HashFromMessage(msg, hash)
	}

	btcsig := new(btc.Signature)
	var sb [65]byte
	sb[0] = 27
	if signkey.IsCompressed() {
		sb[0] += 4
	}

	r, s, e := btc.EcdsaSign(signkey.Key, hash)
	if e != nil {
		println(e.Error())
		return
	}
	btcsig.R.Set(r)
	btcsig.S.Set(s)

	rd := btcsig.R.Bytes()
	sd := btcsig.S.Bytes()
	copy(sb[1+32-len(rd):], rd)
	copy(sb[1+64-len(sd):], sd)

	rpk := btcsig.RecoverPublicKey(hash[:], 0)
	sa := btc.NewAddrFromPubkey(rpk.Bytes(signkey.IsCompressed()), signkey.BtcAddr.Version)
	if sa.Hash160==signkey.BtcAddr.Hash160 {
		fmt.Println(base64.StdEncoding.EncodeToString(sb[:]))
		return
	}

	rpk = btcsig.RecoverPublicKey(hash[:], 1)
	sa = btc.NewAddrFromPubkey(rpk.Bytes(signkey.IsCompressed()), signkey.BtcAddr.Version)
	if sa.Hash160==signkey.BtcAddr.Hash160 {
		sb[0]++
		fmt.Println(base64.StdEncoding.EncodeToString(sb[:]))
		return
	}
	println("Something went wrong. The message has not been signed.")
}
