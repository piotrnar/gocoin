package main

import (
	"fmt"
	"github.com/piotrnar/gocoin/btc"
	"io/ioutil"
	"github.com/piotrnar/gocoin/others/utils"
	"os"
)


func main() {
	fmt.Println("Gocoin FetchTx version", btc.SourcesTag)

	if len(os.Args) < 2 {
		fmt.Println("Specify transaction id on the command line (MSB).")
		return
	}

	txid := btc.NewUint256FromString(os.Args[1])
	rawtx := utils.GetTxFromWeb(txid)
	if rawtx==nil {
		fmt.Println("Error fetching the transaction")
	} else {
		ioutil.WriteFile(txid.String()+".tx", rawtx, 0666)
	}
}
