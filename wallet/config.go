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
	keycnt uint = 50
	testnet bool = false
	waltype uint = 3
	type2sec string
	uncompressed bool = false
	fee string = "0.00001"
	apply2bal bool = true
)

func parse_config() {
	cfgfn := os.Getenv("GOCOIN_WALLET_CONFIG")
	if cfgfn=="" {
		cfgfn = "wallet.cfg"
	}
	d, e := ioutil.ReadFile(cfgfn)
	if e != nil {
		fmt.Println("wallet.cfg not found")
		return
	}
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
					if v>=1 && v<=3 {
						waltype = uint(v)
					} else {
						println(i, "wallet.cfg: incorrect wallet type", v)
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
					if v>1 {
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

		}
	}

	flag.UintVar(&keycnt, "n", keycnt, "Set the number of keys to be used")
	flag.BoolVar(&testnet, "t", testnet, "Force work with testnet addresses")
	flag.UintVar(&waltype, "type", waltype, "Type of deterministic wallet (1, 2 or 3)")
	flag.StringVar(&type2sec, "t2sec", type2sec, "Enforce using this secret for Type-2 wallet (hex encoded)")
	flag.BoolVar(&uncompressed, "u", uncompressed, "Use uncompressed public keys (not advised)")
	flag.StringVar(&fee, "fee", fee, "Specify transaction fee to be used")
	flag.BoolVar(&apply2bal, "a", apply2bal, "Apply changes to the balance folder")
}
