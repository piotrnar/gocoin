package wallet

import (
	"os"
	"fmt"
	"bytes"
	"io/ioutil"
	"github.com/piotrnar/gocoin/btc"
	"github.com/piotrnar/gocoin/qdb"
	"github.com/piotrnar/gocoin/others/utils"
	"github.com/piotrnar/gocoin/client/common"
)

type pendingSI struct {
	db *qdb.DB
	k qdb.KeyType
	rec *btc.OneWalkRecord
}

var StealthSecrets [][]byte
var newStealthIndexes []pendingSI


func FreeStealthSecrets() {
	for i:=range StealthSecrets {
		utils.ClearBuffer(StealthSecrets[i])
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
				d := utils.GetRawData(dir+fis[i].Name())
				if len(d)!=32 {
					fmt.Println("Error reading key from", dir+fis[i].Name(), len(d))
				} else {
					StealthSecrets = append(StealthSecrets, d)
				}
			}
		}
	} else {
		println("ioutil.ReadDir", er.Error())
	}
	if len(StealthSecrets)==0 {
		fmt.Println("Place secrets of your stealth keys in", dir)
	} else {
		fmt.Println(len(StealthSecrets), "stealth keys found in", dir)
	}
	return
}


func FindStealthSecret(sa *btc.StealthAddr) (d []byte) {
	for i := range StealthSecrets {
		if bytes.Equal(btc.PublicFromPrivate(StealthSecrets[i], true), sa.ScanKey[:]) {
			return StealthSecrets[i]
		}
	}
	return
}


// It is assumed that you call this function onlu after rec.IsStealthIdx() was true
func CheckStealthRec(db *qdb.DB, k qdb.KeyType, rec *btc.OneWalkRecord,
	sa *btc.StealthAddr, d []byte) (fl uint32, uo *btc.OneUnspentTx) {
	sth_scr := rec.Script()
	if sa.CheckNonce(sth_scr[3:]) {
		vo := rec.VOut() // get the spending output
		spend_v := db.GetNoMutex(qdb.KeyType(uint64(k) ^ uint64(vo) ^ uint64(vo+1)))
		if spend_v!=nil {
			rec = btc.NewWalkRecord(spend_v)

			if rec.IsP2KH() {
				var h160 [20]byte
				c := btc.StealthDH(sth_scr[7:40], d)
				spen_exp := btc.DeriveNextPublic(sa.SpendKeys[0][:], c)
				btc.RimpHash(spen_exp, h160[:])
				if bytes.Equal(rec.Script()[3:23], h160[:]) {
					adr := btc.NewAddrFromHash160(h160[:], btc.AddrVerPubkey(common.CFG.Testnet))
					uo = rec.ToUnspent(adr)
					adr.StealthAddr = sa
					uo.StealthC = c
					uo.DestinationAddr = "@"+uo.DestinationAddr // mark as stealth
				}
			} else {
				fl = btc.WALK_NOMORE
			}
		} else {
			fl = btc.WALK_NOMORE
		}
	}
	return
}


func StealthNotify(db *qdb.DB, k qdb.KeyType, rec *btc.OneWalkRecord) {
	println("new stealth index")
	newStealthIndexes = append(newStealthIndexes, pendingSI{db:db, k:k, rec:rec})
}


// Go through all the stealth indexes found in the last block
func BlockAccepted() {
	if len(newStealthIndexes) > 0 {
		println(len(newStealthIndexes), "new stealth outputs found")
		FetchStealthKeys()
		newStealthIndexes = nil
	}
}
