package main

import (
	"fmt"
	"github.com/piotrnar/gocoin/btc"
	"io/ioutil"
	"github.com/piotrnar/gocoin/tools/utils"
	"os"
)


func main() {
	fmt.Println("Gocoin FetchTx version", btc.SourcesTag)

	if len(os.Args) < 2 {
		fmt.Println("Specify transaction id on the command line (MSB).")
		return
	}

	txid := btc.NewUint256FromString(os.Args[1])
	rawtx, brokentx := utils.GetTxFromExplorer(txid)
	if rawtx==nil {
		fmt.Println("Error fetching the transaction")
		if brokentx!=nil {
			ioutil.WriteFile("bad-"+txid.String()+".tx", brokentx, 0666)
		}
	} else {
		ioutil.WriteFile(txid.String()+".tx", rawtx, 0666)
	}
}
