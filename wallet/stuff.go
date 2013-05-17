package main

import (
	"os"
	"fmt"
	"bufio"
	"math/big"
	"strings"
	"crypto/rand"
	"crypto/ecdsa"
	"encoding/hex"
	"github.com/piotrnar/gocoin/btc"
)


// Get TxOut record, by the given TxPrevOut
func UO(uns *btc.TxPrevOut) *btc.TxOut {
	tx, _ := loadedTxs[uns.Hash]
	return tx.TxOut[uns.Vout]
}


// Read a line from stdin
func getline() string {
	li, _, _ := bufio.NewReader(os.Stdin).ReadLine()
	return string(li)
}


func ask_yes_no(msg string) bool {
	for {
		fmt.Print(msg, " (y/n) : ")
		l := strings.ToLower(getline())
		if l=="y" {
			return true
		} else if l=="n" {
			return false
		}
	}
	return false
}

// Input the password (that is the secret seed to your wallet)
func getpass() string {
	f, e := os.Open("wallet.sec")
	if e != nil {
		fmt.Println("Seed file 'wallet.sec' not found.")
		fmt.Print("Enter your wallet's seed password: ")
		pass := getline()
		if pass!="" && *dump {
			fmt.Print("Re-enter the seed password (to be sure): ")
			if pass!=getline() {
				println("The two passwords you entered do not match")
				os.Exit(1)
			}
			// Maybe he wants to save the password?
			if ask_yes_no("Save the password on disk, so you won't be asked for it later?") {
				f, e = os.Create("wallet.sec")
				if e == nil {
					f.Write([]byte(pass))
					f.Close()
				} else {
					println("Could not save the password:", e.Error())
				}
			}
		}
		return pass
	}
	le, _ := f.Seek(0, os.SEEK_END)
	buf := make([]byte, le)
	f.Seek(0, os.SEEK_SET)
	n, e := f.Read(buf[:])
	if e != nil {
		println("Reading secret file:", e.Error())
		os.Exit(1)
	}
	if int64(n)!=le {
		println("Something is wrong with the password file. Aborting.")
		os.Exit(1)
	}
	for i := range buf {
		if buf[i]<' ' || buf[i]>126 {
			fmt.Println("WARNING: Your secret contains non-printable characters\007")
			break
		}
	}
	return string(buf)
}

// Verify the secret key's range and al if a test message signed with it verifies OK
func verify_key(priv []byte, publ []byte) bool {
	const TestMessage = "Just some test message..."
	hash := btc.Sha2Sum([]byte(TestMessage))

	pub_key, e := btc.NewPublicKey(publ)
	if e != nil {
		println("NewPublicKey:", e.Error(), "\007")
		os.Exit(1)
	}

	var key ecdsa.PrivateKey
	key.D = new(big.Int).SetBytes(priv)
	key.PublicKey = pub_key.PublicKey

	if key.D.Cmp(big.NewInt(0)) == 0 {
		println("pubkey value is zero")
		return false
	}

	if key.D.Cmp(maxKeyVal) != -1 {
		println("pubkey value is too big", hex.EncodeToString(publ))
		return false
	}

	r, s, err := ecdsa.Sign(rand.Reader, &key, hash[:])
	if err != nil {
		println("ecdsa.Sign:", err.Error())
		return false
	}

	ok := ecdsa.Verify(&key.PublicKey, hash[:], r, s)
	if !ok {
		println("The key pair does not verify!")
		return false
	}
	return true
}
