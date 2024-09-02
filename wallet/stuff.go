package main

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"strings"

	"github.com/piotrnar/gocoin/lib/btc"
	"github.com/piotrnar/gocoin/lib/others/ltc"
	"github.com/piotrnar/gocoin/lib/others/sys"
)

// loadedTxs is a cache for txs from already loaded from balance/ folder.
var loadedTxs map[[32]byte]*btc.Tx = make(map[[32]byte]*btc.Tx)

// getline reads a line from stdin.
func getline() string {
	li, _, _ := bufio.NewReader(os.Stdin).ReadLine()
	return string(li)
}

func ask_yes_no(msg string) bool {
	for {
		fmt.Print(msg, " (y/n) : ")
		l := strings.ToLower(getline())
		if l == "y" {
			return true
		} else if l == "n" {
			return false
		}
	}
}

func no_sign_mode() bool {
	return *list || *dumpwords || *dumpxprv
}

func getpass() ([]byte, error) {
	var pass [1024]byte
	var n int
	var e error
	var f *os.File

	if stdin {
		if *ask4pass {
			return nil, errors.New("both -p and -stdin switches are not allowed")
		}
		d, er := io.ReadAll(os.Stdin)
		if er != nil {
			return nil, er
		}
		n = len(d)
		if n <= 0 {
			return nil, errors.New("empty seed password provided via stdin")
		}
		copy(pass[:n], d)
		sys.ClearBuffer(d)
		goto check_pass
	}

	if !*ask4pass {
		f, e = os.Open(PassSeedFilename)
		if e == nil {
			n, _ = f.Read(pass[:])
			f.Close()
			if n <= 0 {
				return nil, errors.New("empty seed file " + PassSeedFilename)
			}
			goto check_pass
		}

		fmt.Println("Seed file", PassSeedFilename, "not found")
	}

	fmt.Print("Enter your wallet's seed password: ")
	n = sys.ReadPassword(pass[:])
	if n <= 0 {
		return nil, errors.New("empty seed password entered by the user")
	}

	if no_sign_mode() {
		if !*singleask {
			fmt.Print("Re-enter the seed password (to be sure): ")
			var pass2 [1024]byte
			p2len := sys.ReadPassword(pass2[:])
			if p2len != n || !bytes.Equal(pass[:n], pass2[:p2len]) {
				sys.ClearBuffer(pass[:n])
				sys.ClearBuffer(pass2[:p2len])
				return nil, errors.New("two passwords entered by the user do not match")
			}
			sys.ClearBuffer(pass2[:p2len])
		}
		if !*ask4pass {
			// Maybe he wants to save the password?
			if ask_yes_no("Save the password on disk, so you won't be asked for it later?") {
				e = os.WriteFile(PassSeedFilename, pass[:n], 0600)
				if e != nil {
					fmt.Println("WARNING: Could not save the password", e.Error())
				} else {
					fmt.Println("The seed password has been stored in", PassSeedFilename)
				}
			}
		}
	}
check_pass:
	for i := 0; i < n; i++ {
		if pass[i] < ' ' || pass[i] > 126 {
			fmt.Println("WARNING: Your secret contains non-printable characters")
			break
		}
	}
	outpass := make([]byte, n+len(secret_seed))
	if len(secret_seed) > 0 {
		copy(outpass, secret_seed)
	}
	copy(outpass[len(secret_seed):], pass[:n])
	sys.ClearBuffer(pass[:n])
	return outpass, nil
}

// get_change_addr returns the change addrress or nil if there will be no change.
func get_change_addr() (chng *btc.BtcAddr) {
	if *change != "" {
		var e error
		chng, e = btc.NewAddrFromString(*change)
		if e != nil {
			println("Change address:", e.Error())
			cleanExit(1)
		}
		assert_address_version(chng)
		return
	}

	// If change address not specified, send it back to the first input
	for idx := range unspentOuts {
		uo := getUO(&unspentOuts[idx].TxPrevOut)
		if k := pkscr_to_key(uo.Pk_script); k != nil {
			chng = btc.NewAddrFromPkScript(uo.Pk_script, testnet)
			//chng = k.BtcAddr
			return
		}
	}

	fmt.Println("ERROR: Could not determine address where to send change. Add -change switch")
	cleanExit(1)
	return
}

func raw_tx_from_file(fn string) *btc.Tx {
	dat := sys.GetRawData(fn)
	if dat == nil {
		fmt.Println("Cannot fetch raw transaction data")
		return nil
	}
	tx, txle := btc.NewTx(dat)
	if tx != nil {
		tx.SetHash(dat)
		if txle != len(dat) {
			fmt.Println("WARNING: Raw transaction length mismatch", txle, len(dat))
		}
	}
	return tx
}

// tx_from_balance gets the tx for the given ID from the balance folder, or from cache.
func tx_from_balance(txid *btc.Uint256, error_is_fatal bool) (tx *btc.Tx) {
	if tx = loadedTxs[txid.Hash]; tx != nil {
		return // we have it in cache already
	}
	fn := "balance/" + txid.String() + ".tx"
	buf, er := ioutil.ReadFile(fn)
	if er == nil && buf != nil {
		tx, _ = btc.NewTx(buf)
		if error_is_fatal && tx == nil {
			println("Transaction is corrupt:", txid.String())
			cleanExit(1)
		}
		tx.SetHash(buf)
		if txid.Hash != tx.Hash.Hash {
			println("Transaction ID mismatch:", txid.String(), tx.Hash.String())
			cleanExit(1)
		}
	} else if error_is_fatal {
		println("Error reading transaction file:", fn)
		if er != nil {
			println(er.Error())
		}
		cleanExit(1)
	}
	loadedTxs[txid.Hash] = tx // store it in the cache
	return
}

// getUO looks for specific TxPrevOut in the balance folder.
func getUO(pto *btc.TxPrevOut) *btc.TxOut {
	if _, ok := loadedTxs[pto.Hash]; !ok {
		loadedTxs[pto.Hash] = tx_from_balance(btc.NewUint256(pto.Hash[:]), true)
	}
	return loadedTxs[pto.Hash].TxOut[pto.Vout]
}

// ver_pubkey returns the version byte for P2KH addresses.
func ver_pubkey() byte {
	if litecoin {
		return ltc.AddrVerPubkey(testnet)
	} else {
		return btc.AddrVerPubkey(testnet)
	}
}

// ver_script returns the version byte for P2SH addresses.
func ver_script() byte {
	if litecoin {
		return ltc.AddrVerScript(testnet)
	} else {
		return btc.AddrVerScript(testnet)
	}
}

// ver_secret returns the version byte for private key addresses.
func ver_secret() byte {
	return ver_pubkey() + 0x80
}

// addr_from_pkscr gets the BtcAddr from pk_script.
func addr_from_pkscr(scr []byte) *btc.BtcAddr {
	if litecoin {
		return ltc.NewAddrFromPkScript(scr, testnet)
	} else {
		return btc.NewAddrFromPkScript(scr, testnet)
	}
}

// assert_address_version makes sure the version byte in the given address is what we expect.
func assert_address_version(a *btc.BtcAddr) {
	if a.SegwitProg != nil {
		if a.SegwitProg.HRP != btc.GetSegwitHRP(testnet) {
			println("Sending address", a.String(), "has an incorrect HRP string", a.SegwitProg.HRP)
			cleanExit(1)
		}
	} else if a.Version != ver_pubkey() && a.Version != ver_script() {
		println("Sending address", a.String(), "has an incorrect version", a.Version)
		cleanExit(1)
	}
}
