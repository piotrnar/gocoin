package main

import (
	"os"
	"fmt"
	"strconv"
	"strings"
	"io/ioutil"
	"github.com/piotrnar/gocoin/btc"
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
					*testnet = v
				} else {
					println(i, "wallet.cfg: value error for", ll[0], ":", e.Error())
				}

			case "type":
				v, e := strconv.ParseUint(ll[1], 10, 32)
				if e == nil {
					if v>=1 && v<=3 {
						*waltype = uint(v)
					} else {
						println(i, "wallet.cfg: incorrect wallet type", v)
					}
				} else {
					println(i, "wallet.cfg: value error for", ll[0], ":", e.Error())
				}

			case "type2sec":
				*type2sec = ll[1]

			case "keycnt":
				v, e := strconv.ParseUint(ll[1], 10, 32)
				if e == nil {
					if v>1 {
						*keycnt = uint(v)
					} else {
						println(i, "wallet.cfg: incorrect key count", v)
					}
				} else {
					println(i, "wallet.cfg: value error for", ll[0], ":", e.Error())
				}

			case "uncompressed":
				v, e := strconv.ParseBool(ll[1])
				if e == nil {
					*uncompressed = v
				} else {
					println(i, "wallet.cfg: value error for", ll[0], ":", e.Error())
				}

			// case "secrand": <-- deprecated

			case "fee":
				fi, e := btc.StringToSatoshis(ll[1])
				if e != nil {
					println("wallet.cfg: Incorrect fee value", ll[1])
					os.Exit(1)
				}
				curFee = fi

			case "apply2bal":
				v, e := strconv.ParseBool(ll[1])
				if e == nil {
					*apply2bal = v
				} else {
					println(i, "wallet.cfg: value error for", ll[0], ":", e.Error())
				}

			case "secret":
				PassSeedFilename = ll[1]

			case "others":
				RawKeysFilename = ll[1]

		}
	}
	if *fee!="" {
		fi, e := btc.StringToSatoshis(*fee)
		if e != nil {
			println("Incorrect fee value", *fee)
			os.Exit(1)
		}
		curFee = fi
	}
}
