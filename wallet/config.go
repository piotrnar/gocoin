package main

import (
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
)

var (
	keycnt       uint   = 250
	testnet      bool   = false
	waltype      uint   = 3
	uncompressed bool   = false
	fee          string = "0.001"
	apply2bal    bool   = true
	secret_seed  []byte
	litecoin     bool = false
	txfilename   string
	stdin        bool
	hdpath       string = "m/0'"
	bip39wrds    int    = 0
	minsig       bool
	usescrypt    uint
	hdsubs       uint   = 1
	atype        string = "p2kh"

	segwit_mode, bech32_mode, taproot_mode, pubkeys_mode bool
)

func check_atype() {
	switch atype {
	case "p2kh":
		segwit_mode, bech32_mode, taproot_mode, pubkeys_mode = false, false, false, false
	case "segwit":
		segwit_mode, bech32_mode, taproot_mode, pubkeys_mode = true, false, false, false
	case "bech32":
		segwit_mode, bech32_mode, taproot_mode, pubkeys_mode = true, true, false, false
	case "tap":
		segwit_mode, bech32_mode, taproot_mode, pubkeys_mode = true, true, true, false
	case "pks":
		segwit_mode, bech32_mode, taproot_mode, pubkeys_mode = false, false, false, true
	default:
		println("ERROR: Invalid value of atype:", atype)
		os.Exit(1)
	}
}

func parse_config() {
	cfgfn := ""

	// pre-parse command line: look for -cfg <fname> or -h
	for i := 1; i < len(os.Args); i++ {
		if os.Args[i] == "-cfg" || os.Args[i] == "--cfg" {
			if i+1 >= len(os.Args) {
				println("Missing the file name for", os.Args[i], "argument")
				os.Exit(1)
			}
			cfgfn = os.Args[i+1]
			break
		}
		if strings.HasPrefix(os.Args[i], "-cfg=") || strings.HasPrefix(os.Args[i], "--cfg=") {
			ss := strings.SplitN(os.Args[i], "=", 2)
			cfgfn = ss[1]
		}
	}

	if cfgfn == "" {
		cfgfn = os.Getenv("GOCOIN_WALLET_CONFIG")
		if cfgfn == "" {
			cfgfn = *cfg_fn
		}
	}
	d, e := os.ReadFile(cfgfn)
	if e != nil {
		fmt.Println(cfgfn, "not found - proceeding with the default config values.")
	} else {
		fmt.Println("Using config file", cfgfn)
		lines := strings.Split(string(d), "\n")
		for i := range lines {
			line := strings.Trim(lines[i], " \n\r\t")
			if len(line) == 0 || line[0] == '#' {
				continue
			}

			ll := strings.SplitN(line, "=", 2)
			if len(ll) != 2 {
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
					if v >= 1 && v <= 4 {
						waltype = uint(v)
					} else {
						println(i, "wallet.cfg: incorrect wallet type", v)
						os.Exit(1)
					}
				} else {
					println(i, "wallet.cfg: value error for", ll[0], ":", e.Error())
					os.Exit(1)
				}

			case "hdpath":
				hdpath = strings.Trim(ll[1], "\"")

			case "bip39":
				v, e := strconv.ParseInt(ll[1], 10, 32)
				if e == nil {
					if v == -1 || v >= 12 && v <= 24 && (v%3) == 0 {
						bip39wrds = int(v)
					} else {
						println(i, "wallet.cfg: incorrect bip39 value", v)
						os.Exit(1)
					}
				} else {
					println(i, "wallet.cfg: value error for", ll[0], ":", e.Error())
					os.Exit(1)
				}

			case "hdsubs":
				v, e := strconv.ParseUint(ll[1], 10, 32)
				if e == nil {
					if v >= 1 {
						hdsubs = uint(v)
					} else {
						println(i, "wallet.cfg: incorrect hdsubs value", v)
						os.Exit(1)
					}
				} else {
					println(i, "wallet.cfg: value error for", ll[0], ":", e.Error())
					os.Exit(1)
				}

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

			case "atype":
				atype = strings.Trim(ll[1], "\" ")

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

			case "minsig":
				v, e := strconv.ParseBool(ll[1])
				if e == nil {
					minsig = v
				} else {
					println(i, "wallet.cfg: value error for", ll[0], ":", e.Error())
					os.Exit(1)
				}

			case "scrypt":
				v, e := strconv.ParseUint(ll[1], 10, 32)
				if e == nil {
					if v >= 1 {
						usescrypt = uint(v)
					} else {
						println(i, "wallet.cfg: incorrect scrypt value", v)
						os.Exit(1)
					}
				} else {
					println(i, "wallet.cfg: value error for", ll[0], ":", e.Error())
					os.Exit(1)
				}

			case "rfc6979":
				v, e := strconv.ParseBool(ll[1])
				if e == nil {
					*rfc6979 = v
				} else {
					println(i, "wallet.cfg: value error for", ll[0], ":", e.Error())
					os.Exit(1)
				}

			case "prompt":
				v, e := strconv.ParseBool(ll[1])
				if e == nil {
					*prompt = v
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
	flag.StringVar(&hdpath, "hdpath", hdpath, "Derivation Path to the first key in HD wallet (type=4)")
	flag.UintVar(&hdsubs, "hdsubs", hdsubs, "Create HD Wallet with so many sub-accounts (use 2 for common walets)")
	flag.IntVar(&bip39wrds, "bip39", bip39wrds, "Create HD Wallet in BIP39 mode using 12, 15, 18, 21 or 24 words")
	flag.BoolVar(&uncompressed, "u", uncompressed, "Deprecated in this version")
	flag.StringVar(&fee, "fee", fee, "Specify transaction fee to be used")
	flag.BoolVar(&apply2bal, "a", apply2bal, "Apply changes to the balance folder (does not work with -raw)")
	flag.BoolVar(&litecoin, "ltc", litecoin, "Litecoin mode")
	flag.StringVar(&txfilename, "txfn", "", "Use this filename for output transaction (otherwise use a random name)")
	flag.BoolVar(&stdin, "stdin", stdin, "Read password from stdin")
	flag.BoolVar(&minsig, "minsig", minsig, "Make sure R and S inside ECDSA signatures are only 32 bytes long")
	flag.UintVar(&usescrypt, "scrypt", usescrypt, "Use extra scrypt function to convert password into private keys (default 0 = disabled)")
	flag.StringVar(&atype, "atype", atype, "When listing, use this type of deposit addresses")
	if uncompressed {
		fmt.Println("WARNING: Using uncompressed keys")
	}
}
