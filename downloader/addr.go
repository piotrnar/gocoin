package main

import (
	"time"
	"bytes"
	"math/rand"
	"github.com/piotrnar/gocoin/lib/qdb"
	"github.com/piotrnar/gocoin/lib/btc"
	"github.com/piotrnar/gocoin/lib/others/sys"
	"github.com/piotrnar/gocoin/lib/others/peersdb"
)


func get_best_peer() (peer *peersdb.PeerAddr) {
	adrs := peersdb.GetBestPeers(100, is_connected)
	if len(adrs)==0 {
		return nil
	}
	return adrs[rand.Int31n(int32(len(adrs)))]
}


func parse_addr(pl []byte) {
	b := bytes.NewBuffer(pl)
	cnt, _ := btc.ReadVLen(b)
	for i := 0; i < int(cnt); i++ {
		var buf [30]byte
		n, e := b.Read(buf[:])
		if n!=len(buf) || e!=nil {
			COUNTER("ADER")
			break
		}
		a := peersdb.NewPeer(buf[:])
		if !sys.ValidIp4(a.Ip4[:]) {
			COUNTER("ADNO")
		} else if time.Unix(int64(a.Time), 0).Before(time.Now().Add(time.Minute)) {
			if time.Now().Before(time.Unix(int64(a.Time), 0).Add(peersdb.ExpirePeerAfter)) {
				k := qdb.KeyType(a.UniqID())
				v := peersdb.PeerDB.Get(k)
				if v != nil {
					a.Banned = peersdb.NewPeer(v[:]).Banned
				}
				peersdb.PeerDB.Put(k, a.Bytes())
			} else {
				COUNTER("ADST")
			}
		} else {
			COUNTER("ADFU")
		}
	}
}

func open_connection_count() (res uint) {
	open_connection_mutex.Lock()
	res = uint(len(open_connection_list))
	open_connection_mutex.Unlock()
	return
}
