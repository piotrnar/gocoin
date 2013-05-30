package main

import (
	"fmt"
	"sync"
	"github.com/piotrnar/gocoin/btc"
)

var (
	alerts map[int32] *btc.Alert = make(map[int32] *btc.Alert)
	alert_access sync.Mutex
	alertPubKey []byte
)

func (c *oneConnection) HandleAlert(b []byte) {
	a, e := btc.NewAlert(b, alertPubKey)
	if e != nil {
		println(c.PeerAddr.String(), "- alert:", e.Error())
		c.DoS()
		return
	}
	alert_access.Lock()
	if _, ok := alerts[a.ID]; !ok {
		alerts[a.ID] = a
		fmt.Println("\007New alert:", a.StatusBar)
		ui_show_prompt()
	}
	alert_access.Unlock()
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