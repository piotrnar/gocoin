package common

import (
	"bytes"
	"io/ioutil"
	"encoding/json"
	"github.com/piotrnar/gocoin/lib/btc"
)

type oneMinerId struct {
	Name string
	Tag []byte
}

var MinerIds []oneMinerId


func MinedBy(bl []byte, tag []byte) bool {
	max2search := 0x200
	if len(bl)<max2search {
		max2search = len(bl)
	}
	return bytes.Index(bl[0x51:max2search], tag)!=-1
}


func MinedByUs(bl []byte) bool {
	LockCfg()
	minid := CFG.Beeps.MinerID
	UnlockCfg()
	if minid=="" {
		return false
	}
	return MinedBy(bl, []byte(minid))
}

func BlocksMiner(bl []byte) (string, int) {
	for i, m := range MinerIds {
		if MinedBy(bl, m.Tag) {
			return m.Name, i
		}
	}
	bt, _ := btc.NewBlock(bl)
	cbtx, _ := btc.NewTx(bl[bt.TxOffset:])
	adr := btc.NewAddrFromPkScript(cbtx.TxOut[0].Pk_script, Testnet)
	if adr!=nil {
		return adr.String(), -1
	}
	return "", -1
}

func ReloadMiners() {
	d, _ := ioutil.ReadFile("miners.json")
	if d!=nil {
		var MinerIdFile [][3]string
		e := json.Unmarshal(d, &MinerIdFile)
		if e != nil {
			println("miners.json", e.Error())
			return
		}
		MinerIds = nil
		for _, r := range MinerIdFile {
			var rec oneMinerId
			rec.Name = r[0]
			if r[1]!="" {
				rec.Tag = []byte(r[1])
				MinerIds = append(MinerIds, rec)
			} else {
				if a, _ := btc.NewAddrFromString(r[2]); a != nil {
					rec.Tag = a.OutScript()
					MinerIds = append(MinerIds, rec)
				}
			}
		}
	}
}
