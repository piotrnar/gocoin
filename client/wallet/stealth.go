package wallet

import (
	"os"
	"fmt"
	"bytes"
	"io/ioutil"
	"github.com/piotrnar/gocoin/lib/btc"
	"github.com/piotrnar/gocoin/lib/others/sys"
	"github.com/piotrnar/gocoin/client/common"
)


type stealthCacheRec struct {
	h160 [20]byte
	addr *btc.BtcAddr
	d [32]byte
}

var (
	ArmedStealthSecrets [][]byte
	StealthSecrets [][]byte

	StealthAdCache []stealthCacheRec
)

func FreeStealthSecrets() {
	for i:=range StealthSecrets {
		sys.ClearBuffer(StealthSecrets[i])
	}
	StealthSecrets = nil
}

func FetchStealthKeys() {
	FreeStealthSecrets()
	dir := common.GocoinHomeDir+"wallet"+string(os.PathSeparator)+"stealth"+string(os.PathSeparator)
	fis, er := ioutil.ReadDir(dir)
	if er == nil {
		for i := range fis {
			if !fis[i].IsDir() && fis[i].Size()>=32 {
				d := sys.GetRawData(dir+fis[i].Name())
				if len(d)!=32 {
					fmt.Println("Error reading key from", dir+fis[i].Name(), len(d))
				} else {
					StealthSecrets = append(StealthSecrets, d)
				}
			}
		}
	} else {
		//println("ioutil.ReadDir", er.Error())
		os.MkdirAll(dir, 0700)
	}

	if !PrecachingComplete {
		if len(StealthSecrets)==0 {
			fmt.Println("Place secrets of your stealth keys in", dir, " (use 'arm' to load more)")
		} else {
			fmt.Println(len(StealthSecrets), "stealth keys found in", dir, " (use 'arm' to load more)")
		}
	}
	return
}


func FindStealthSecret(sa *btc.StealthAddr) (d []byte) {
	for i := range StealthSecrets {
		if bytes.Equal(btc.PublicFromPrivate(StealthSecrets[i], true), sa.ScanKey[:]) {
			return StealthSecrets[i]
		}
	}
	for i := range ArmedStealthSecrets {
		if bytes.Equal(btc.PublicFromPrivate(ArmedStealthSecrets[i], true), sa.ScanKey[:]) {
			return ArmedStealthSecrets[i]
		}
	}
	return
}
