// This tool outpus Type-2 deterministic addresses, as described here:
// https://bitcointalk.org/index.php?topic=19137.0
// At input it takes "A_public_key" and "B_secret" - both values as hex encoded strings.
// Optionally, you can add a third parameter - number of public keys you want to calculate.
package main

import (
	"os"
	"fmt"
	"encoding/hex"
	"github.com/piotrnar/gocoin/lib/btc"
)


func main() {
	if len(os.Args) < 3 {
		fmt.Println("Specify secret and public_key to get the next Type-2 deterministic address")
		fmt.Println("Add -t as the third argument to work with Testnet addresses.")
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

	testnet := len(os.Args) > 3 && os.Args[3]=="-t"

	// Old address
	public_key = btc.DeriveNextPublic(public_key, secret)

	// New address
	fmt.Println(btc.NewAddrFromPubkey(public_key, btc.AddrVerPubkey(testnet)).String())
	// New key
	fmt.Println(hex.EncodeToString(public_key))

}
