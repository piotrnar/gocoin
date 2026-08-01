package main

import (
	"encoding/hex"
	"fmt"
	"os"
	"sort"
	"time"

	"github.com/piotrnar/gocoin/client/peersdb"
	"github.com/piotrnar/gocoin/lib/others/qdb"
	"github.com/piotrnar/gocoin/lib/others/sys"
)

func main() {
	var dir string

	if len(os.Args) > 1 {
		dir = os.Args[1]
	} else {
		dir = sys.BitcoinHome() + "gocoin" + string(os.PathSeparator) + "btcnet" + string(os.PathSeparator) + "peers3"
	}

	db, er := qdb.NewDB(dir, true)

	if er != nil {
		println(er.Error())
		os.Exit(1)
	}

	println(db.Count(), "peers in databse", dir)
	if db.Count() == 0 {
		return
	}

	tmp := make(peersdb.ManyPeers, 0, db.Count())
	db.Browse(func(k qdb.KeyType, v []byte) uint32 {
		np := peersdb.NewPeer(v)
		tmp = append(tmp, np)
		return 0
	})

	sort.Sort(tmp)
	for cnt := range tmp {
		ad := tmp[cnt]
		fmt.Printf("%5d) %16s   %5d  - seen %5d min ago    %s\n", cnt+1,
			fmt.Sprintf("%d.%d.%d.%d", ad.Ip4[0], ad.Ip4[1], ad.Ip4[2], ad.Ip4[3]),
			ad.Port, (time.Now().Unix()-int64(ad.Time))/60, hex.EncodeToString(ad.Ip6[:]))
	}
}
