package main

import (
	"fmt"
	"os"
	"sort"
	"time"

	"github.com/piotrnar/gocoin/client/network/peersdb"
	"github.com/piotrnar/gocoin/lib/others/qdb"
	"github.com/piotrnar/gocoin/lib/others/sys"
)

type manyPeers []*peersdb.PeerAddr

func (mp manyPeers) Len() int {
	return len(mp)
}

func (mp manyPeers) Less(i, j int) bool {
	return mp[i].Time > mp[j].Time
}

func (mp manyPeers) Swap(i, j int) {
	mp[i], mp[j] = mp[j], mp[i]
}

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

	tmp := make(manyPeers, db.Count())
	cnt := 0
	db.Browse(func(k qdb.KeyType, v []byte) uint32 {
		np := peersdb.NewPeer(v)
		if !sys.ValidIp4(np.Ip4[:]) {
			return 0
		}
		if cnt < len(tmp) {
			tmp[cnt] = np
			cnt++
		}
		return 0
	})

	sort.Sort(tmp[:cnt])
	for cnt = 0; cnt < len(tmp) && cnt < 2500; cnt++ {
		ad := tmp[cnt]
		fmt.Printf("%3d) %16s   %5d  - seen %5d min ago\n", cnt+1,
			fmt.Sprintf("%d.%d.%d.%d", ad.Ip4[0], ad.Ip4[1], ad.Ip4[2], ad.Ip4[3]),
			ad.Port, (time.Now().Unix()-int64(ad.Time))/60)
	}
}
