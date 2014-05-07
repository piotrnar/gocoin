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


func CheckStealthRec(db *qdb.DB, k qdb.KeyType, rec *btc.OneWalkRecord,
	sa *btc.StealthAddr, d []byte) (fl uint32, uo *btc.OneUnspentTx) {
	if rec.IsStealthIdx() {
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
						uo = rec.ToUnspent(btc.NewAddrFromHash160(h160[:], btc.AddrVerPubkey(common.CFG.Testnet)))
						uo.StealthC = c
					}
				} else {
					fl = btc.WALK_NOMORE
				}
			} else {
				fl = btc.WALK_NOMORE
			}
		}
	}
	return
}
