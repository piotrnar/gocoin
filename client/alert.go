package main

import (
	"fmt"
	"sync"
	"encoding/binary"
	"github.com/piotrnar/gocoin/btc"
)

var (
	alerts map[uint64] *btc.Alert = make(map[uint64] *btc.Alert)
	alert_access sync.Mutex
	alertPubKey []byte  // set in init.go
)

func (c *oneConnection) HandleAlert(b []byte) {
	var rh [20]byte
	btc.RimpHash(b, rh[:])
	alidx := binary.LittleEndian.Uint64(rh[0:8])

	alert_access.Lock() // protect access to the map while in the function
	defer alert_access.Unlock()

	if _, ok := alerts[alidx]; ok {
		return // already have this one
	}

	a, e := btc.NewAlert(b, alertPubKey)
	if e != nil {
		println(c.PeerAddr.String(), "- sent us a broken alert:", e.Error())
		c.DoS()
		return
	}

	alerts[alidx] = a
	fmt.Println("\007New alert:", a.StatusBar)
	ui_show_prompt()
	return
}


func list_alerst(p string) {
	alert_access.Lock()
	for _, v := range alerts {
		fmt.Println(v.Version, v.RelayUntil, v.Expiration, v.ID, v.Cancel,
			v.MinVer, v.MaxVer, v.Priority, v.Comment, v.StatusBar, v.Reserved)
	}
	alert_access.Unlock()
}


func init() {
	newUi("alerts a", false, list_alerst, "Show received alerts")
}