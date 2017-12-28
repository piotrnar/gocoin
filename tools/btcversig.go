// This tool is able to verify whether a message was signed with the given bitcoin address
package main

import (
	"os"
	"fmt"
	"flag"
	"bytes"
	"io/ioutil"
	"github.com/piotrnar/gocoin/lib/btc"
	"github.com/piotrnar/gocoin/lib/others/ltc"
)


var (
	addr = flag.String("a", "", "base58 encoded bitcoin address that supposedly signed the message (required)")
	sign = flag.String("s", "", "base64 encoded signature of the message (required)")
	mess = flag.String("m", "", "the message (optional)")
	mfil = flag.String("f", "", "the filename containing a signed message (optional)")
	unix = flag.Bool("u", false, "remove all \\r characters from the message (optional)")
	help = flag.Bool("h", false, "print this help")
	verb = flag.Bool("v", false, "verbose mode")
	litecoin = flag.Bool("ltc", false, "litecoin mode")
)

func main() {
	var msg []byte

	flag.Parse()

	if *help || *addr=="" || *sign=="" {
		flag.PrintDefaults()
		return
	}

	ad, er := btc.NewAddrFromString(*addr)
	if !*litecoin && ad!=nil && ad.Version==ltc.AddrVerPubkey(false) {
		*litecoin = true
	}
	if er != nil {
		println("Address:", er.Error())
		flag.PrintDefaults()
		return
	}

	nv, btcsig, er := btc.ParseMessageSignature(*sign)
	if er != nil {
		println("ParseMessageSignature:", er.Error())
		return
	}

	if *mess!="" {
		msg = []byte(*mess)
	} else if *mfil!="" {
		msg, er = ioutil.ReadFile(*mfil)
		if er != nil {
			println(er.Error())
			return
		}
	} else {
		if *verb {
			fmt.Println("Enter the message:")
		}
		msg, _ = ioutil.ReadAll(os.Stdin)
	}

	if *unix {
		if *verb {
			fmt.Println("Enforcing Unix text format")
		}
		msg = bytes.Replace(msg, []byte{'\r'}, nil, -1)
	}

	hash := make([]byte, 32)
	if *litecoin {
		ltc.HashFromMessage(msg, hash)
	} else {
		btc.HashFromMessage(msg, hash)
	}

	compressed := false
	if nv >= 31 {
		if *verb {
			fmt.Println("compressed key")
		}
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
				if bytes.IndexByte(msg, '\r') != -1 {
					fmt.Println("You have CR chars in the message. Try to verify with -u switch.")
				}
				os.Exit(1)
			} else {
				fmt.Println("Signature OK")
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
