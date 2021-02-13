package main

import (
	"os"
	"fmt"
	"flag"
	"strconv"
	"strings"
	"io/ioutil"
)

var (
	keycnt uint = 250
	testnet bool = false
	waltype uint = 3
	type2sec string
	uncompressed bool = false
	fee string = "0.001"
	apply2bal bool = true
	secret_seed []byte
	litecoin bool = false
	txfilename string
	stdin bool
    hdwaltype uint = 0
    bip39bits uint = 0
)

func parse_config() {
	cfgfn := os.Getenv("GOCOIN_WALLET_CONFIG")
	if cfgfn=="" {
		cfgfn = "wallet.cfg"
		fmt.Println("GOCOIN_WALLET_CONFIG not set")
	}
	d, e := ioutil.ReadFile(cfgfn)
	if e != nil {
		fmt.Println(cfgfn, "not found")
	} else {
		fmt.Println("Using config file", cfgfn)
		lines := strings.Split(string(d), "\n")
		for i := range lines {
			line := strings.Trim(lines[i], " \n\r\t")
			if len(line)==0 || line[0]=='#' {
				continue
			}

			ll := strings.SplitN(line, "=", 2)
			if len(ll)!=2 {
				println(i, "wallet.cfg: syntax error in line", ll)
				continue
			}

			switch strings.ToLower(ll[0]) {
				case "testnet":
					v, e := strconv.ParseBool(ll[1])
					if e == nil {
						testnet = v
					} else {
						println(i, "wallet.cfg: value error for", ll[0], ":", e.Error())
						os.Exit(1)
					}

				case "type":
					v, e := strconv.ParseUint(ll[1], 10, 32)
					if e == nil {
						if v>=1 && v<=4 {
							waltype = uint(v)
						} else {
							println(i, "wallet.cfg: incorrect wallet type", v)
							os.Exit(1)
						}
					} else {
						println(i, "wallet.cfg: value error for", ll[0], ":", e.Error())
						os.Exit(1)
					}

				case "hdtype":
					v, e := strconv.ParseUint(ll[1], 10, 32)
					if e == nil {
						if v >= 0 && v <= 4 {
							hdwaltype = uint(v)
						} else {
							println(i, "wallet.cfg: incorrect HD wallet type", v)
							os.Exit(1)
						}
					} else {
						println(i, "wallet.cfg: value error for", ll[0], ":", e.Error())
						os.Exit(1)
					}

				case "bip39":
					v, e := strconv.ParseUint(ll[1], 10, 32)
					if e == nil {
                        if v >= 128 && v <= 256 && (v % 32) == 0 {
							bip39bits = uint(v)
						} else {
							println(i, "wallet.cfg: incorrect bip39 value", v)
							os.Exit(1)
						}
					} else {
						println(i, "wallet.cfg: value error for", ll[0], ":", e.Error())
						os.Exit(1)
					}
                
				case "type2sec":
					type2sec = ll[1]

				case "keycnt":
					v, e := strconv.ParseUint(ll[1], 10, 32)
					if e == nil {
						if v >= 1 {
							keycnt = uint(v)
						} else {
							println(i, "wallet.cfg: incorrect key count", v)
							os.Exit(1)
						}
					} else {
						println(i, "wallet.cfg: value error for", ll[0], ":", e.Error())
						os.Exit(1)
					}

				case "uncompressed":
					v, e := strconv.ParseBool(ll[1])
					if e == nil {
						uncompressed = v
					} else {
						println(i, "wallet.cfg: value error for", ll[0], ":", e.Error())
						os.Exit(1)
					}

				// case "secrand": <-- deprecated

				case "fee":
					fee = ll[1]

				case "apply2bal":
					v, e := strconv.ParseBool(ll[1])
					if e == nil {
						apply2bal = v
					} else {
						println(i, "wallet.cfg: value error for", ll[0], ":", e.Error())
						os.Exit(1)
					}

				case "secret":
					PassSeedFilename = ll[1]

				case "others":
					RawKeysFilename = ll[1]

				case "seed":
					if !*nosseed {
						secret_seed = []byte(strings.Trim(ll[1], " \t\n\r"))
					}

				case "litecoin":
					v, e := strconv.ParseBool(ll[1])
					if e == nil {
						litecoin = v
					} else {
						println(i, "wallet.cfg: value error for", ll[0], ":", e.Error())
						os.Exit(1)
					}

			}
		}
	}

	flag.UintVar(&keycnt, "n", keycnt, "Set the number of determinstic keys to be calculated by the wallet")
	flag.BoolVar(&testnet, "t", testnet, "Testnet mode")
	flag.UintVar(&waltype, "type", waltype, "Type of a deterministic wallet to be used (1 to 4)")
	flag.UintVar(&hdwaltype, "hdtype", hdwaltype, "Type of a deterministic wallet to be used (1 to 4)")
	flag.UintVar(&bip39bits, "bip39", bip39bits, "Create HD Wallet in BIP39 mode (with 128, 160, 192, 224 or 256 entropy bits)")
	flag.StringVar(&type2sec, "t2sec", type2sec, "Enforce using this secret for Type-2 wallet (hex encoded)")
	flag.BoolVar(&uncompressed, "u", uncompressed, "Deprecated in this version")
	flag.StringVar(&fee, "fee", fee, "Specify transaction fee to be used")
	flag.BoolVar(&apply2bal, "a", apply2bal, "Apply changes to the balance folder (does not work with -raw)")
	flag.BoolVar(&litecoin, "ltc", litecoin, "Litecoin mode")
	flag.StringVar(&txfilename, "txfn", "", "Use this filename for output transaction (otherwise use a random name)")
	flag.BoolVar(&stdin, "stdin", stdin, "Read password from stdin")
	if uncompressed {
		fmt.Println("WARNING: Using uncompressed keys")
	}
}
