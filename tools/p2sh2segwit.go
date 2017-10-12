package main

import (
	"os"
	"fmt"
	"bufio"
	"strings"
	"github.com/piotrnar/gocoin/lib/btc"
)

func main() {
	rd := bufio.NewReader(os.Stdin)
	for {
		d, _, _ := rd.ReadLine()
		if d == nil {
			break
		}

		lns := strings.SplitN(strings.Trim(string(d), " \t"), " ", 2)
		if len(lns) < 1 {
			continue
		}

		aa, er := btc.NewAddrFromString(lns[0])
		if aa != nil && er == nil {
			h160 := btc.Rimp160AfterSha256(append([]byte{0,20}, aa.Hash160[:]...))
			aa = btc.NewAddrFromHash160(h160[:], btc.AddrVerScript(false))
			fmt.Print(aa.String())
			if len(lns) > 1 {
				fmt.Print(" ", lns[1])
			}
			fmt.Println()
		}
	}
}
