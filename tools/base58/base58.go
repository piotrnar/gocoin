package main

import (
	"os"
	"fmt"
	"flag"
	"strings"
	"io/ioutil"
	"encoding/hex"
	"github.com/piotrnar/gocoin/lib/btc"
)

var (
	decode = flag.Bool("d", false, "run base58 decode (instead of encode)")
	binary = flag.Bool("b", false, "binary (insted of hex) for decode output")
	help = flag.Bool("h", false, "print this help")
)

func main() {
	flag.Parse()
	if *help {
		flag.PrintDefaults()
		return
	}

	msg, _ := ioutil.ReadAll(os.Stdin)
	if len(msg)==0 {
		return
	}

	if *decode {
		res := btc.Decodeb58(strings.Trim(string(msg), " \t\n\r"))
		if *binary {
			os.Stdout.Write(res)
		} else {
			fmt.Println(hex.EncodeToString(res))
		}
	} else {
		fmt.Println(btc.Encodeb58(msg))
	}
}
