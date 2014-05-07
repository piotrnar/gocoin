package wallet

import (
	"os"
	"fmt"
	"io/ioutil"
	//"encoding/json"
	//"github.com/piotrnar/gocoin/btc"
	"github.com/piotrnar/gocoin/others/utils"
	"github.com/piotrnar/gocoin/client/common"
)

func FetchStealthKeys() (res [][]byte) {
	dir := common.GocoinHomeDir+"wallet"+string(os.PathSeparator)+"stealth"+string(os.PathSeparator)
	fis, er := ioutil.ReadDir(dir)
	if er == nil {
		for i := range fis {
			if !fis[i].IsDir() && fis[i].Size()>=32 {
				d := utils.GetRawData(dir+fis[i].Name())
				if len(d)!=32 {
					fmt.Println("Error reading key from", dir+fis[i].Name(), len(d))
				} else {
					res = append(res, d)
				}
			}
		}
	} else {
		println("ioutil.ReadDir", er.Error())
	}
	if len(res)==0 {
		fmt.Println("Place secrets of your stealth keys in", dir)
	} else {
		fmt.Println(len(res), "stealth keys found in", dir)
	}
	return
}