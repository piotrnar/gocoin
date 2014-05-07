package wallet

import (
	"os"
	"fmt"
	"bytes"
	"io/ioutil"
	"github.com/piotrnar/gocoin/btc"
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


func FindStealthSecret(sa *btc.StealthAddr) (d []byte) {
	ds := FetchStealthKeys()
	if len(ds)==0 {
		return
	}
	for i := range ds {
		if d==nil && bytes.Equal(btc.PublicFromPrivate(ds[i], true), sa.ScanKey[:]) {
			d = ds[i]
		} else {
			utils.ClearBuffer(ds[i])
		}
	}
	return
}
