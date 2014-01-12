// This tool outpus Type-2 deterministic addresses, as described here:
// https://bitcointalk.org/index.php?topic=19137.0
// At input it takes "A_public_key" and "B_secret" - both values as hex encoded strings.
// Optionally, you can add a third parameter - number of public keys you want to calculate.
package main

import (
	"os"
	"fmt"
	"strconv"
	"math/big"
	"encoding/hex"
	"github.com/piotrnar/gocoin/btc"
)


func main() {
	var testnet bool

	if len(os.Args) < 3 {
		fmt.Println("Specify A_public_key and B_secret (and optionaly number of addresses you want)")
		fmt.Println("Use a negative value for number of addresses, to work with Testnet addresses.")
		return
	}
	A_public_key, er := hex.DecodeString(os.Args[1])
	if er != nil {
		println("Error parsing A_public_key:", er.Error())
		os.Exit(1)
	}

	pubk, er := btc.NewPublicKey(A_public_key)
	if er != nil {
		println("Invalid valid public key:", er.Error())
		os.Exit(1)
	}
	compressed := len(A_public_key)==33

	B_secret, er := hex.DecodeString(os.Args[2])
	if er != nil {
		println("Error parsing B_secret:", er.Error())
		os.Exit(1)
	}
	sec := new(big.Int).SetBytes(B_secret)

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
	fmt.Println("#", hex.EncodeToString(pubk.Bytes(compressed)))
	fmt.Println("#", hex.EncodeToString(sec.Bytes()))

	for i:=1; i<=int(n); i++ {
		fmt.Println(btc.NewAddrFromPubkey(pubk.Bytes(compressed), btc.AddrVerPubkey(testnet)).String(), "TypB", i)
		if i >= int(n) {
			break
		}

		pubk.X, pubk.Y = btc.DeriveNextPublic(pubk.X, pubk.Y, sec)
	}
}
