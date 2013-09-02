package main

import (
	"os"
	"fmt"
	"net"
	"time"
	"sync"
	"sort"
	"bytes"
	"errors"
	"strings"
	"strconv"
	"hash/crc64"
	"encoding/binary"
	"github.com/piotrnar/gocoin/qdb"
	"github.com/piotrnar/gocoin/btc"
)

const (
	defragEvery = (5*time.Minute)
	ExpirePeerAfter = (3*time.Hour) // https://en.bitcoin.it/wiki/Protocol_specification#addr
)

var (
	peerDB *qdb.DB
	crctab = crc64.MakeTable(crc64.ISO)

	proxyPeer *onePeer // when this is not nil we should only connect to this single node
	peerdb_mutex sync.Mutex
)

type onePeer struct {
	btc.NetAddr

	Time uint32  // When seen last time
	Banned uint32 // time when this address baned or zero if never
}


func newPeer(v []byte) (p *onePeer) {
	//println("newad:", hex.EncodeToString(v))
	if len(v) < 30 {
		println("newPeer: unexpected length", len(v))
		return
	}
	p = new(onePeer)
	p.Time = binary.LittleEndian.Uint32(v[0:4])
	p.Services = binary.LittleEndian.Uint64(v[4:12])
	copy(p.Ip6[:], v[12:24])
	copy(p.Ip4[:], v[24:28])
	p.Port = binary.BigEndian.Uint16(v[28:30])

	if len(v) == 34 {
		p.Banned = binary.LittleEndian.Uint32(v[30:34])
	}

	return
}


func NewIncommingPeer(ipstr string) (p *onePeer, e error) {
	x := strings.Index(ipstr, ":")
	if x != -1 {
		ipstr = ipstr[:x] // remove port number
	}
	ip := net.ParseIP(ipstr)
	if ip != nil && len(ip)==16 {
		p = new(onePeer)
		copy(p.Ip4[:], ip[12:16])
		p.Services = Services
		copy(p.Ip6[:], ip[:12])
		p.Port = DefaultTcpPort
		if dbp := peerDB.Get(qdb.KeyType(p.UniqID())); dbp!=nil && newPeer(dbp).Banned!=0 {
			e = errors.New(p.Ip() + " is banned")
			p = nil
		} else {
			p.Time = uint32(time.Now().Unix())
			p.Save()
		}
	} else {
		e = errors.New("Error parsing IP '"+ipstr+"'")
	}
	return
}


func (p *onePeer) Bytes() []byte {
	b := new(bytes.Buffer)
	binary.Write(b, binary.LittleEndian, p.Time)
	binary.Write(b, binary.LittleEndian, p.Services)
	b.Write(p.Ip6[:])
	b.Write(p.Ip4[:])
	binary.Write(b, binary.BigEndian, p.Port)
	binary.Write(b, binary.LittleEndian, p.Banned)
	return b.Bytes()
}


func expire_peers() {
	peerdb_mutex.Lock()
	var delcnt uint32
	now := time.Now()
	todel := make([]qdb.KeyType, peerDB.Count())
	peerDB.Browse(func(k qdb.KeyType, v []byte) uint32 {
		ptim := binary.LittleEndian.Uint32(v[0:4])
		if now.After(time.Unix(int64(ptim), 0).Add(ExpirePeerAfter)) {
			todel[delcnt] = k // we cannot call Del() from here
			delcnt++
		}
		return 0
	})
	if delcnt > 0 {
		CountSafeAdd("PeersExpired", uint64(delcnt))
		for delcnt > 0 {
			delcnt--
			peerDB.Del(todel[delcnt])
		}
		CountSafe("PeerDefragsDone")
		peerDB.Defrag()
	} else {
		CountSafe("PeerDefragsNone")
	}
	peerdb_mutex.Unlock()
}


func (p *onePeer) Save() {
	peerDB.Put(qdb.KeyType(p.UniqID()), p.Bytes())
}


func (p *onePeer) Ban() {
	p.Banned = uint32(time.Now().Unix())
	p.Save()
}


func (p *onePeer) Alive() {
	p.Time = uint32(time.Now().Unix())
	p.Save()
}


func (p *onePeer) Dead() {
	p.Time -= 600 // make it 10 min older
	p.Save()
}


func (p *onePeer) Ip() (string) {
	return fmt.Sprintf("%d.%d.%d.%d:%d", p.Ip4[0], p.Ip4[1], p.Ip4[2], p.Ip4[3], p.Port)
}


func (p *onePeer) String() (s string) {
	s = fmt.Sprintf("%21s", p.Ip())

	now := uint32(time.Now().Unix())
	if p.Banned != 0 {
		s += fmt.Sprintf("  *BAN %3d min ago", (now-p.Banned)/60)
	} else {
		s += fmt.Sprintf("  Seen %3d min ago", (now-p.Time)/60)
	}
	return
}


func (p *onePeer) UniqID() (uint64) {
	h := crc64.New(crctab)
	h.Write(p.Ip6[:])
	h.Write(p.Ip4[:])
	h.Write([]byte{byte(p.Port>>8),byte(p.Port)})
	return h.Sum64()
}


type manyPeers []*onePeer

func (mp manyPeers) Len() int {
	return len(mp)
}

func (mp manyPeers) Less(i, j int) bool {
	return mp[i].Time > mp[j].Time
}

func (mp manyPeers) Swap(i, j int) {
	mp[i], mp[j] = mp[j], mp[i]
}


// Discard any IP that may refer to a local network
func ValidIp4(ip []byte) bool {
	// local host
	if ip[0]==0 || ip[0]==127 {
		return false
	}

	// RFC1918
	if ip[0]==10 || ip[0]==192 && ip[1]==168 || ip[0]==172 && ip[1]>=16 && ip[1]<=31 {
		return false
	}

	//RFC3927
	if ip[0]==169 && ip[1]==254 {
		return false
	}

	return true
}

// Fetch a given number of best (most recenty seen) peers.
// Set unconnected to true to only get those that we are not connected to.
func GetBestPeers(limit uint, unconnected bool) (res manyPeers) {
	if proxyPeer!=nil {
		if !unconnected || !connectionActive(proxyPeer) {
			return manyPeers{proxyPeer}
		}
		return manyPeers{}
	}
	peerdb_mutex.Lock()
	tmp := make(manyPeers, 0)
	peerDB.Browse(func(k qdb.KeyType, v []byte) uint32 {
		ad := newPeer(v)
		if ad.Banned==0 && ValidIp4(ad.Ip4[:]) {
			if !unconnected || !connectionActive(ad) {
				tmp = append(tmp, ad)
			}
		}
		return 0
	})
	peerdb_mutex.Unlock()
	// Copy the top rows to the result buffer
	if len(tmp)>0 {
		sort.Sort(tmp)
		if uint(len(tmp))<limit {
			limit = uint(len(tmp))
		}
		res = make(manyPeers, limit)
		copy(res, tmp[:limit])
	}
	return
}


func initSeeds(seeds []string, port uint16) {
	for i := range seeds {
		ad, er := net.LookupHost(seeds[i])
		if er == nil {
			for j := range ad {
				ip := net.ParseIP(ad[j])
				if ip != nil && len(ip)==16 {
					p := new(onePeer)
					p.Time = uint32(time.Now().Unix())
					p.Services = 1
					copy(p.Ip6[:], ip[:12])
					copy(p.Ip4[:], ip[12:16])
					p.Port = port
					p.Save()
				}
			}
		} else {
			println("initSeeds LookupHost", seeds[i], "-", er.Error())
		}
	}
}


// shall be called from the main thread
func initPeers(dir string) {
	peerDB, _ = qdb.NewDB(dir+"peers3", true)

	if CFG.ConnectOnly != "" {
		x := strings.Index(CFG.ConnectOnly, ":")
		if x == -1 {
			CFG.ConnectOnly = fmt.Sprint(CFG.ConnectOnly, ":", DefaultTcpPort)
		}
		oa, e := net.ResolveTCPAddr("tcp4", CFG.ConnectOnly)
		if e != nil {
			println(e.Error())
			os.Exit(1)
		}
		proxyPeer = new(onePeer)
		proxyPeer.Services = 1
		copy(proxyPeer.Ip4[:], oa.IP[0:4])
		proxyPeer.Port = uint16(oa.Port)
		fmt.Printf("Connect to bitcoin network via %d.%d.%d.%d:%d\n",
			oa.IP[0], oa.IP[1], oa.IP[2], oa.IP[3], oa.Port)
	} else {
		newUi("pers", false, show_addresses, "Dump pers database (warning: may be long)")
		go func() {
			if !CFG.Testnet {
				initSeeds([]string{"seed.bitcoin.sipa.be", "dnsseed.bluematt.me",
					/*"dnsseed.bitcoin.dashjr.org",*/ "bitseed.xf2.org"}, 8333)
			} else {
				initSeeds([]string{"bitcoin.petertodd.org", "testnet-seed.bitcoin.petertodd.org",
					/*"bluematt.me",*/ "testnet-seed.bluematt.me"}, 18333)
			}
		}()
	}
}


func show_addresses(par string) {
	fmt.Println(peerDB.Count(), "peers in the database")
	if par=="list" {
		cnt :=  0
		peerDB.Browse(func(k qdb.KeyType, v []byte) uint32 {
			cnt++
			fmt.Printf("%4d) %s\n", cnt, newPeer(v).String())
			return 0
		})
	} else if par=="ban" {
		cnt :=  0
		peerDB.Browse(func(k qdb.KeyType, v []byte) uint32 {
			pr := newPeer(v)
			if pr.Banned != 0 {
				cnt++
				fmt.Printf("%4d) %s\n", cnt, pr.String())
			}
			return 0
		})
		if cnt==0 {
			fmt.Println("No banned peers in the DB")
		}
	} else if par != "" {
		limit, er := strconv.ParseUint(par, 10, 32)
		if er != nil {
			fmt.Println("Specify number of best peers to display")
			return
		}
		prs := GetBestPeers(uint(limit), false)
		for i := range prs {
			fmt.Printf("%4d) %s", i+1, prs[i].String())
			if connectionActive(prs[i]) {
				fmt.Print("  CONNECTED")
			}
			fmt.Print("\n")
		}
	} else {
		fmt.Println("Use 'peers list' to list them")
	}
}

func ClosePeerDB() {
	if peerDB!=nil {
		fmt.Println("Closing peer DB")
		peerDB.Sync()
		peerDB.Defrag()
		peerDB.Close()
		peerDB = nil
	}
}
