// This tool is able to verify whether a message was signed with the given bitcoin address
package main

import (
	"os"
	"fmt"
	"flag"
	"strings"
	"io/ioutil"
	"github.com/piotrnar/gocoin/lib/btc"
)


var (
	addr = flag.String("a", "", "base58 encoded bitcoin address that supposedly signed the message (required)")
	sign = flag.String("s", "", "base64 encoded signature of the message (required)")
	mess = flag.String("m", "", "the message (optional)")
	mfil = flag.String("f", "", "the filename containing a signed message (optional)")
	unix = flag.Bool("u", false, "remove all \\r characters from the message (optional)")
	help = flag.Bool("h", false, "print this help")
)

func main() {
	var msg []byte

	flag.Parse()

	if *help || *addr=="" || *sign=="" {
		flag.PrintDefaults()
		return
	}

	ad, er := btc.NewAddrFromString(*addr)
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
		fmt.Println("Enter the message:")
		msg, _ = ioutil.ReadAll(os.Stdin)
	}

	if *unix {
		fmt.Println("Enforcing Unix text format")
		msg = []byte(strings.Replace(string(msg), "\r", "", -1))
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
