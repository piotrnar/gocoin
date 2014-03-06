// This tool outpus Type-2 deterministic addresses, as described here:
// https://bitcointalk.org/index.php?topic=19137.0
// At input it takes "A_public_key" and "B_secret" - both values as hex encoded strings.
// Optionally, you can add a third parameter - number of public keys you want to calculate.
package main

import (
	"os"
	"fmt"
	"math/big"
	"encoding/hex"
	"github.com/piotrnar/gocoin/btc"
)


func main() {
	if len(os.Args) < 3 {
		fmt.Println("Specify B_secret and A_public_key to get the next Type-2 deterministic address")
		fmt.Println("Add -t as the third argument to work with Testnet addresses.")
		return
	}
	A_public_key, er := hex.DecodeString(os.Args[2])
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

	B_secret, er := hex.DecodeString(os.Args[1])
	if er != nil {
		println("Error parsing B_secret:", er.Error())
		os.Exit(1)
	}
	sec := new(big.Int).SetBytes(B_secret)

	testnet := len(os.Args) > 3 && os.Args[3]=="-t"

	// Old address
	fmt.Print(btc.NewAddrFromPubkey(pubk.Bytes(compressed), btc.AddrVerPubkey(testnet)).String(), " => ")
	pubk.X, pubk.Y = btc.DeriveNextPublic(pubk.X, pubk.Y, sec)

	// New address
	fmt.Println(btc.NewAddrFromPubkey(pubk.Bytes(compressed), btc.AddrVerPubkey(testnet)).String())
	// New key
	fmt.Println(hex.EncodeToString(pubk.Bytes(compressed)))

}
