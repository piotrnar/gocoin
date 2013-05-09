package main

import (
	"fmt"
	"sync"
	"github.com/piotrnar/gocoin/btc"
)

var (
	alerts map[int32] *btc.Alert = make(map[int32] *btc.Alert)
	alert_access sync.Mutex
)

func (c *oneConnection) HandleAlert(b []byte) {
	a, e := btc.NewAlert(b)
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
