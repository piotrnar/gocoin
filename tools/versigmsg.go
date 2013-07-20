// This tool is able to verify whether a message was signed with the given bitcoin address
package main

import (
	"os"
	"fmt"
	"io/ioutil"
	"github.com/piotrnar/gocoin/btc"
)

func main() {
	if len(os.Args) < 3 {
		fmt.Println("Specify at least two parameters:")
		fmt.Println(" 1) The base58 encoded bitcoin addres, that the signature was made with")
		fmt.Println(" 2) The base64 encoded signature for the message...")
		fmt.Println("If you specify a 3rd parameter - this will be assumed to be the message you want to verify")
		fmt.Println("If you do not specify a 3rd parameter - the message will be read from stdin")
		return
	}
	ad, er := btc.NewAddrFromString(os.Args[1])
	if er != nil {
		println("Address:", er.Error())
		return
	}

	nv, btcsig, er := btc.ParseMessageSignature(os.Args[2])
	if er != nil {
		println("ParseMessageSignature:", er.Error())
		return
	}

	var msg []byte
	if len(os.Args) < 4 {
		msg, _ = ioutil.ReadAll(os.Stdin)
	} else {
		msg = []byte(os.Args[3])
	}

	hash := make([]byte, 32)
	btc.HashFromMessage(msg, hash)

	compressed := false
	if nv >= 31 {
		//println("compressed key")
		nv -= 4
		compressed = true
	}

	pub := btcsig.RecoverPublicKey(hash[:], int(nv-27))
	if pub != nil {
		pk := pub.Bytes(compressed)
		ok := btc.EcdsaVerify(pk, btcsig.Bytes(), hash)
		if ok {
			sa := btc.NewAddrFromPubkey(pk, ad.Version)
			if ad.Hash160!=sa.Hash160 {
				fmt.Println("BAD signature for", ad.String())
				os.Exit(1)
			} else {
				fmt.Println("Good signature for", sa.String())
			}
		} else {
			println("BAD signature")
			os.Exit(1)
		}
	} else {
		println("BAD, BAD, BAD signature")
		os.Exit(1)
	}
}
