package main

import (
	"fmt"
	"github.com/piotrnar/gocoin/lib/btc"
	"io/ioutil"
	"github.com/piotrnar/gocoin/lib/others/utils"
	"github.com/piotrnar/gocoin"
	"os"
)


func main() {
	fmt.Println("Gocoin FetchBlock version", gocoin.Version)

	if len(os.Args) < 2 {
		fmt.Println("Specify block hash on the command line (MSB).")
		return
	}

	hash := btc.NewUint256FromString(os.Args[1])
	bl := utils.GetBlockFromWeb(hash)
	if bl==nil {
		fmt.Println("Error fetching the block")
	} else {
		ioutil.WriteFile(bl.Hash.String()+".bin", bl.Raw, 0666)
	}
}
