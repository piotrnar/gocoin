package network

import (
	"sync"
	"encoding/hex"
	"encoding/binary"
	"github.com/piotrnar/gocoin/btc"
)

var (
	Alerts map[uint64] *btc.Alert = make(map[uint64] *btc.Alert)
	Alert_access sync.Mutex
	AlertPubKey []byte  // set in init.go
	NetAlerts chan string = make(chan string, 1)
)

func (c *OneConnection) HandleAlert(b []byte) {
	var rh [20]byte
	btc.RimpHash(b, rh[:])
	alidx := binary.LittleEndian.Uint64(rh[0:8])

	Alert_access.Lock() // protect access to the map while in the function
	defer Alert_access.Unlock()

	if _, ok := Alerts[alidx]; ok {
		return // already have this one
	}

	a, e := btc.NewAlert(b, AlertPubKey)
	if e != nil {
		println(c.PeerAddr.String(), "- sent us a broken alert:", e.Error())
		if a == nil {
			//println("With apparently broken signature - so ban it!")
			c.DoS("BrokenAlert")
		} else {
			println(hex.EncodeToString(b))
		}
		return
	}

	Alerts[alidx] = a
	NetAlerts <- a.StatusBar
	return
}
