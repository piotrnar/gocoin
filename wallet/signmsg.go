package main

import (
	"os"
	"fmt"
	"io/ioutil"
	"encoding/hex"
	"encoding/base64"
	"github.com/piotrnar/gocoin/lib/btc"
	"github.com/piotrnar/gocoin/lib/ltc"
)


// this function signs either a message or a raw transaction hash
func sign_message() {
	var hash []byte

	if *signhash!="" {
		var er error
		hash, er = hex.DecodeString(*signhash)
		if er != nil {
			println("Incorrect content of -hash parameter")
			println(er.Error())
			return
		}
	}

	ad2s, e := btc.NewAddrFromString(*signaddr)
	if e != nil {
		println(e.Error())
		if *signhash!="" {
			println("Always use -sign <addr> along with -hash <msghash>")
		}
		return
	}

	var privkey []byte
	var compr bool

	for i := range keys {
		if keys[i].BtcAddr.Hash160==ad2s.Hash160 {
			privkey = keys[i].Key
			compr = keys[i].BtcAddr.IsCompressed()

			// Sign raw hash?
			if hash!=nil {
				txsig := new(btc.Signature)
				txsig.HashType = 0x01
				r, s, e := btc.EcdsaSign(privkey, hash)
				if e != nil {
					println(e.Error())
					return
				}
				txsig.R.Set(r)
				txsig.S.Set(s)
				fmt.Println("PublicKey:", hex.EncodeToString(keys[i].BtcAddr.Pubkey))
				fmt.Println(hex.EncodeToString(txsig.Bytes()))
				return
			}

			break
		}
	}
	if privkey==nil {
		println("You do not have a private key for", ad2s.String())
		return
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
	if compr {
		sb[0] += 4
	}

	r, s, e := btc.EcdsaSign(privkey, hash)
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
	sa := btc.NewAddrFromPubkey(rpk.Bytes(compr), ad2s.Version)
	if sa.Hash160==ad2s.Hash160 {
		fmt.Println(base64.StdEncoding.EncodeToString(sb[:]))
		return
	}

	rpk = btcsig.RecoverPublicKey(hash[:], 1)
	sa = btc.NewAddrFromPubkey(rpk.Bytes(compr), ad2s.Version)
	if sa.Hash160==ad2s.Hash160 {
		sb[0]++
		fmt.Println(base64.StdEncoding.EncodeToString(sb[:]))
		return
	}
	println("Something went wrong. The message has not been signed.")
}
