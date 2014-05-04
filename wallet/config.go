package main

import (
	"os"
	"fmt"
	"strconv"
	"strings"
	"io/ioutil"
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
				v, e := strconv.ParseFloat(ll[1], 64)
				if e == nil {
					if v>=0 {
						*fee = v
					} else {
						println(i, "wallet.cfg: incorrect fee value", v)
					}
				} else {
					println(i, "wallet.cfg: value error for", ll[0], ":", e.Error())
				}

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
}
