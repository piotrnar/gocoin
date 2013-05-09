package main

import (
	"fmt"
	"github.com/piotrnar/gocoin/btc"
)

func (c *oneConnection) HandleAlert(b []byte) {
	a, e := btc.NewAlert(b)
	if e != nil {
		println(c.PeerAddr.String(), "- alert:", e.Error())
		c.DoS()
		return
	}
	fmt.Println("\007Alert:", a.StatusBar)
	ui_show_prompt()
	return
}
