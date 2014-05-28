package main

import (
	"os"
	"fmt"
	"bufio"
	"bytes"
	"strings"
	"io/ioutil"
	"github.com/piotrnar/gocoin/lib/btc"
	"github.com/piotrnar/gocoin/lib/ltc"
	"github.com/piotrnar/gocoin/lib/others/sys"
)


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
func getseed(seed []byte) bool {
	var pass [1024]byte
	var n int
	var e error
	var f *os.File

	if !*ask4pass {
		f, e = os.Open(PassSeedFilename)
		if e == nil {
			n, e = f.Read(pass[:])
			f.Close()
			if n <= 0 {
				return false
			}
			goto calc_seed
		}

		fmt.Println("Seed file", PassSeedFilename, "not found")
	}

	fmt.Print("Enter your wallet's seed password: ")
	n = sys.ReadPassword(pass[:])
	if n<=0 {
		return false
	}

	if *list || *scankey!="" {
		if !*singleask {
			fmt.Print("Re-enter the seed password (to be sure): ")
			var pass2 [1024]byte
			p2len := sys.ReadPassword(pass2[:])
			if p2len!=n || !bytes.Equal(pass[:n], pass2[:p2len]) {
				sys.ClearBuffer(pass2[:p2len])
				println("The two passwords you entered do not match")
				return false
			}
			sys.ClearBuffer(pass2[:p2len])
		}
		if *list {
			// Maybe he wants to save the password?
			if ask_yes_no("Save the password on disk, so you won't be asked for it later?") {
				e = ioutil.WriteFile(PassSeedFilename, pass[:n], 0600)
				if e != nil {
					fmt.Println("WARNING: Could not save the password", e.Error())
				} else {
					fmt.Println("The seed password has been stored in", PassSeedFilename)
				}
			}
		}
	}
calc_seed:
	for i:=0; i<n; i++ {
		if pass[i]<' ' || pass[i]>126 {
			fmt.Println("WARNING: Your secret contains non-printable characters")
			break
		}
	}
	if len(secret_seed)>0 {
		x := append(secret_seed, pass[:n]...)
		sys.ClearBuffer(secret_seed)
		btc.ShaHash(x, seed)
		sys.ClearBuffer(x)
	} else {
		btc.ShaHash(pass[:n], seed)
	}
	sys.ClearBuffer(pass[:n])
	return true
}


// return the change addrress or nil if there will be no change
func get_change_addr() (chng *btc.BtcAddr) {
	if *change!="" {
		var e error
		chng, e = btc.NewAddrFromString(*change)
		if e != nil {
			println("Change address:", e.Error())
			os.Exit(1)
		}
		assert_address_version(chng)
		return
	}

	// If change address not specified, send it back to the first input
	for idx := range unspentOuts {
		if unspentOuts[idx].stealth {
			continue // cannot send change to a stelath address since we don't know the scankey
		}
		uo := UO(unspentOuts[idx])
		if k := pkscr_to_key(uo.Pk_script); k!=nil {
			chng = k.BtcAddr
			return
		}
	}

	fmt.Println("ERROR: Could not determine address where to send change. Add -change switch")
	os.Exit(1)
	return
}


func raw_tx_from_file(fn string) *btc.Tx {
	dat := sys.GetRawData(fn)
	if dat==nil {
		fmt.Println("Cannot fetch raw transaction data")
		return nil
	}
	tx, txle := btc.NewTx(dat)
	if tx != nil {
		tx.Hash = btc.NewSha2Hash(dat)
		if txle != len(dat) {
			fmt.Println("WARNING: Raw transaction length mismatch", txle, len(dat))
		}
	}
	return tx
}


func tx_from_balance(txid *btc.Uint256, error_is_fatal bool) (tx *btc.Tx) {
	fn := "balance/" + txid.String() + ".tx"
	buf, er := ioutil.ReadFile(fn)
	if er==nil && buf!=nil {
		var th [32]byte
		btc.ShaHash(buf, th[:])
		if txid.Hash==th {
			tx, _ = btc.NewTx(buf)
			if error_is_fatal && tx == nil {
				println("Transaction is corrupt:", txid.String())
				os.Exit(1)
			}
		} else if error_is_fatal {
			println("Transaction file is corrupt:", txid.String())
			os.Exit(1)
		}
	} else if error_is_fatal {
		println("Error reading transaction file:", fn)
		if er != nil {
			println(er.Error())
		}
		os.Exit(1)
	}
	return
}

func ver_pubkey() byte {
	if litecoin {
		return ltc.AddrVerPubkey(testnet)
	} else {
		return btc.AddrVerPubkey(testnet)
	}
}

func ver_script() byte {
	// for litecoin the version is identical
	return btc.AddrVerScript(testnet)
}

func ver_stealth() byte {
	return btc.StealthAddressVersion(testnet)
}

func ver_secret() byte {
	return ver_pubkey() + 0x80
}

func addr_from_pkscr(scr []byte) *btc.BtcAddr {
	if litecoin {
		return ltc.NewAddrFromPkScript(scr, testnet)
	} else {
		return btc.NewAddrFromPkScript(scr, testnet)
	}
}

func assert_address_version(a *btc.BtcAddr) {
	if a.Version!=ver_pubkey() && a.Version!=ver_script() && a.Version!=ver_stealth() {
		println("Sending address", a.String(), "has an incorrect version", a.Version)
		os.Exit(1)
	}
}
