// This tool outpus Type-2 deterministic addresses, as described here:
// https://bitcointalk.org/index.php?topic=19137.0
// At input it takes "A_public_key" and "secret" - both values as hex encoded strings.
// Optionally, you can add a third parameter - number of public keys you want to calculate.
package main

import (
	"os"
	"fmt"
	"strconv"
	"encoding/hex"
	"github.com/piotrnar/gocoin/lib/btc"
)


func main() {
	var testnet bool

	if len(os.Args) < 3 {
		fmt.Println("Specify secret, public_key and optionaly number of addresses you want.")
		fmt.Println("Use a negative value for number of addresses, to work with Testnet addresses.")
		return
	}
	public_key, er := hex.DecodeString(os.Args[2])
	if er != nil {
		println("Error parsing public_key:", er.Error())
		os.Exit(1)
	}

	if len(public_key)==33 && (public_key[0]==2 || public_key[0]==3) {
		fmt.Println("Compressed")
	} else if len(public_key)==65 && (public_key[0]==4) {
		fmt.Println("Uncompressed")
	} else {
		println("Incorrect public key")
	}

	secret, er := hex.DecodeString(os.Args[1])
	if er != nil {
		println("Error parsing secret:", er.Error())
		os.Exit(1)
	}

	n := int64(25)

	if len(os.Args) > 3 {
		n, er = strconv.ParseInt(os.Args[3], 10, 32)
		if er != nil {
			println("Error parsing number of keys value:", er.Error())
			os.Exit(1)
		}
		if n == 0 {
			return
		}

		if n < 0 {
			n = -n
			testnet = true
		}
	}

	fmt.Println("# Type-2")
	fmt.Println("#", hex.EncodeToString(public_key))
	fmt.Println("#", hex.EncodeToString(secret))

	for i:=1; i<=int(n); i++ {
		fmt.Println(btc.NewAddrFromPubkey(public_key, btc.AddrVerPubkey(testnet)).String(), "TypB", i)
		if i >= int(n) {
			break
		}

		public_key = btc.DeriveNextPublic(public_key, secret)
	}
}
