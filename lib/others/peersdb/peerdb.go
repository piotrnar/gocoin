package peersdb

import (
	"errors"
	"fmt"
	"net"
	"os"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/piotrnar/gocoin/lib/others/qdb"
	"github.com/piotrnar/gocoin/lib/others/sys"
	"github.com/piotrnar/gocoin/lib/others/utils"
)

const (
	ExpireAlivePeerAfter  = (24 * time.Hour) // https://en.bitcoin.it/wiki/Protocol_specification#addr
	ExpireBannedPeerAfter = (7 * 24 * time.Hour)
	MinPeersInDB          = 4096  // Do not expire peers if we have less than this
	MaxPeersInDB          = 65536 // 64k records
)

var (
	PeerDB       *qdb.DB
	proxyPeer    *PeerAddr // when this is not nil we should only connect to this single node
	peerdb_mutex sync.Mutex

	Testnet     bool
	ConnectOnly string
	Services    uint64 = 1
)

type PeerAddr struct {
	*utils.OnePeer

	// The fields below don't get saved, but are used internaly
	Manual bool // Manually connected (from UI)
	Friend bool // Connected from friends.txt

	lastAliveSaved int64 // update the record only once per minute
}

func DefaultTcpPort() uint16 {
	if Testnet {
		return 18333
	} else {
		return 8333
	}
}

func NewEmptyPeer() (p *PeerAddr) {
	p = new(PeerAddr)
	p.OnePeer = new(utils.OnePeer)
	p.Time = uint32(time.Now().Unix() - 600) // Create empty peers with the time 10 minutes in the past
	return
}

func NewPeer(v []byte) (p *PeerAddr) {
	p = new(PeerAddr)
	p.OnePeer = utils.NewPeer(v)
	return
}

func NewAddrFromString(ipstr string, force_default_port bool) (p *PeerAddr, e error) {
	port := DefaultTcpPort()
	x := strings.Index(ipstr, ":")
	if x != -1 {
		if !force_default_port {
			v, er := strconv.ParseUint(ipstr[x+1:], 10, 32)
			if er != nil {
				e = er
				return
			}
			if v > 0xffff {
				e = errors.New("Port number too big")
				return
			}
			port = uint16(v)
		}
		ipstr = ipstr[:x] // remove port number
	}
	ipa, er := net.ResolveIPAddr("ip", ipstr)
	if er == nil {
		if ipa != nil {
			p = NewEmptyPeer()
			p.Services = Services
			p.Port = port
			if len(ipa.IP) == 4 {
				copy(p.Ip4[:], ipa.IP[:])
			} else if len(ipa.IP) == 16 {
				copy(p.Ip4[:], ipa.IP[12:16])
				copy(p.Ip6[:], ipa.IP[:12])
			}
			if dbp := PeerDB.Get(qdb.KeyType(p.UniqID())); dbp != nil {
				p = NewPeer(dbp) // if we already had it, just update the Time field
			}
			p.Time = uint32(time.Now().Unix())
			p.Save()
		} else {
			e = errors.New("peerdb.NewAddrFromString(" + ipstr + ") - unspecified error")
		}
	} else {
		e = errors.New("peerdb.NewAddrFromString(" + ipstr + ") - " + er.Error())
	}
	return
}

func NewPeerFromString(ipstr string, force_default_port bool) (p *PeerAddr, e error) {
	p, e = NewAddrFromString(ipstr, force_default_port)
	if e != nil {
		return
	}

	if sys.IsIPBlocked(p.Ip4[:]) {
		e = errors.New(ipstr + " is blocked")
		return
	}

	if dbp := PeerDB.Get(qdb.KeyType(p.UniqID())); dbp != nil && NewPeer(dbp).Banned != 0 {
		e = errors.New(p.Ip() + " is banned")
		p = nil
	} else {
		p.Time = uint32(time.Now().Unix())
		p.Save()
	}
	return
}

func ExpirePeers() {
	peerdb_mutex.Lock()

	// if DB is full, remove the oldest records
	if PeerDB.Count() > MaxPeersInDB {
		expire_alive_before_time := uint32(time.Now().Add(-ExpireAlivePeerAfter).Unix())
		expire_banned_before_time := uint32(time.Now().Add(-ExpireBannedPeerAfter).Unix())
		recs := make(manyPeers, PeerDB.Count())
		var i, c1, c2, c3 int
		PeerDB.Browse(func(k qdb.KeyType, v []byte) uint32 {
			recs[i] = NewPeer(v)
			return 0
		})
		sort.Sort(recs)
		for i = len(recs) - 1; i > MinPeersInDB; i-- {
			var delit bool
			rec := recs[i]
			if !rec.SeenAlive {
				delit = true
				c1++
			} else if rec.Time < expire_alive_before_time {
				if rec.Banned == 0 {
					delit = true
					c2++
				} else if rec.Banned < expire_banned_before_time {
					delit = true
					c3++
				}
			}
			if delit {
				PeerDB.Del(qdb.KeyType(rec.UniqID()))
				if PeerDB.Count() < MaxPeersInDB {
					break
				}
			}
		}
		PeerDB.Defrag(false)
		peerdb_mutex.Unlock()
		fmt.Print("ExpirePeers deleted ", c1, " untried, ", c2, " alive and ", c3, "banned. Left: ", PeerDB.Count())
		return
	}
	/*
		var delcnt uint32
		now := time.Now()
		todel := make([]qdb.KeyType, PeerDB.Count())
		PeerDB.Browse(func(k qdb.KeyType, v []byte) uint32 {
			ptim := binary.LittleEndian.Uint32(v[0:4])
			if now.After(time.Unix(int64(ptim), 0).Add(ExpirePeerAfter)) || ptim > uint32(now.Unix()+3600) {
				todel[delcnt] = k // we cannot call Del() from here
				delcnt++
			}
			return 0
		})
		if delcnt > 0 {
			for delcnt > 0 && PeerDB.Count() > MinPeersInDB {
				delcnt--
				PeerDB.Del(todel[delcnt])
			}
			PeerDB.Defrag(false)
		}
		peerdb_mutex.Unlock()
	*/
}

func (p *PeerAddr) Save() {
	PeerDB.Put(qdb.KeyType(p.UniqID()), p.Bytes())
	//PeerDB.Sync()
}

func (p *PeerAddr) Ban(reason string) {
	p.Banned = uint32(time.Now().Unix())

	if reason == "" {
		_, fil, line, _ := runtime.Caller(1)
		reason = fmt.Sprint(fil, ":", line)
	}
	if len(reason) > 255 {
		p.BanReason = reason[:255]
	} else {
		p.BanReason = reason
	}
	p.Save()
}

func (p *PeerAddr) Alive() {
	now := time.Now().Unix()
	p.Time = uint32(now)
	if !p.SeenAlive || now-p.lastAliveSaved >= 60 {
		p.SeenAlive = true
		p.lastAliveSaved = now
		p.Save()
	}
}

func (p *PeerAddr) Dead() {
	if !p.SeenAlive && p.Banned == 0 && PeerDB.Count() > MinPeersInDB {
		PeerDB.Del(qdb.KeyType(p.UniqID()))
		return
	}
	p.Time = uint32(time.Now().Unix() - 5*60) // make it last alive 5 minutes ago
	p.Save()
}

func (p *PeerAddr) Ip() string {
	return fmt.Sprintf("%d.%d.%d.%d:%d", p.Ip4[0], p.Ip4[1], p.Ip4[2], p.Ip4[3], p.Port)
}

func (p *PeerAddr) String() (s string) {
	s = fmt.Sprintf("%21s  ", p.Ip())
	if p.Services == 0xffffffffffffffff {
		s += "Srv:ALL"
	} else {
		s += fmt.Sprintf("Srv:%3x", p.Services)
	}

	now := uint32(time.Now().Unix())
	if p.SeenAlive {
		s += "  Alive"
	} else {
		s += "       "
	}
	s += fmt.Sprintf(" %.1f min", float64(int(now)-int(p.Time))/60.0)

	if p.Banned != 0 {
		s += fmt.Sprintf("  BAN %.2f hrs", float64(int(now)-int(p.Banned))/3600.0)
		if p.BanReason != "" {
			s += " because " + p.BanReason
		}
	}

	return
}

type manyPeers []*PeerAddr

func (mp manyPeers) Len() int {
	return len(mp)
}

func (mp manyPeers) Less(i, j int) bool {
	return mp[i].Time > mp[j].Time
}

func (mp manyPeers) Swap(i, j int) {
	mp[i], mp[j] = mp[j], mp[i]
}

func GetRecentPeers(limit uint, ignorePeer func(*PeerAddr) bool) (res manyPeers) {
	return GetRecentPeersExt(limit, true, ignorePeer)
}

// GetRecentPeersExt fetches a given number of best (most recenty seen) peers.
func GetRecentPeersExt(limit uint, sort_result bool, ignorePeer func(*PeerAddr) bool) (res manyPeers) {
	if proxyPeer != nil {
		if ignorePeer == nil || !ignorePeer(proxyPeer) {
			return manyPeers{proxyPeer}
		}
		return manyPeers{}
	}
	res = make(manyPeers, 0)
	peerdb_mutex.Lock()
	PeerDB.Browse(func(k qdb.KeyType, v []byte) uint32 {
		ad := NewPeer(v)
		if sys.ValidIp4(ad.Ip4[:]) && !sys.IsIPBlocked(ad.Ip4[:]) {
			if ignorePeer == nil || !ignorePeer(ad) {
				res = append(res, ad)
				if !sort_result && len(res) >= int(limit) {
					return qdb.BR_ABORT
				}
			}
		}
		return 0
	})
	peerdb_mutex.Unlock()
	if sort_result && len(res) > 0 {
		sort.Sort(res)
		if int(limit) < len(res) {
			res = res[:int(limit)]
		}
	}
	return
}

func initSeeds(seeds []string, port uint16) {
	for i := range seeds {
		ad, er := net.LookupHost(seeds[i])
		if er == nil {
			//println(len(ad), "addrs from", seeds[i])
			for j := range ad {
				ip := net.ParseIP(ad[j])
				if ip != nil && len(ip) == 16 {
					p := NewEmptyPeer()
					p.Services = 0xFFFFFFFFFFFFFFFF
					copy(p.Ip6[:], ip[:12])
					copy(p.Ip4[:], ip[12:16])
					p.Port = port
					if dbp := PeerDB.Get(qdb.KeyType(p.UniqID())); dbp != nil {
						_p := NewPeer(dbp)
						_p.Time = p.Time
						_p.Save() // if we already had it, only update the time field
					} else {
						p.Save()
					}
				}
			}
		} else {
			println("initSeeds LookupHost", seeds[i], "-", er.Error())
		}
	}
}

// InitPeers should be called from the main thread.
func InitPeers(dir string) {
	PeerDB, _ = qdb.NewDB(dir+"peers3", true)

	if ConnectOnly != "" {
		x := strings.Index(ConnectOnly, ":")
		if x == -1 {
			ConnectOnly = fmt.Sprint(ConnectOnly, ":", DefaultTcpPort())
		}
		oa, e := net.ResolveTCPAddr("tcp4", ConnectOnly)
		if e != nil {
			println(e.Error(), ConnectOnly)
			os.Exit(1)
		}
		proxyPeer = NewEmptyPeer()
		proxyPeer.Services = Services
		copy(proxyPeer.Ip4[:], oa.IP[12:16])
		proxyPeer.Port = uint16(oa.Port)
		fmt.Printf("Connect to bitcoin network via %d.%d.%d.%d:%d\n",
			proxyPeer.Ip4[0], proxyPeer.Ip4[1], proxyPeer.Ip4[2], proxyPeer.Ip4[3], proxyPeer.Port)
	} else {
		go func() {
			if !Testnet {
				initSeeds([]string{
					"seed.bitcoin.sipa.be",
					"dnsseed.bluematt.me",
					"dnsseed.bitcoin.dashjr.org",
					"seed.bitcoinstats.com",
					"seed.bitcoin.jonasschnelli.ch",
					"seed.btc.petertodd.org",
					"seed.bitcoin.sprovoost.nl",
					"seed.bitnodes.io",
					"dnsseed.emzy.de",
					"seed.bitcoin.wiz.biz",
				}, 8333)
			} else {
				initSeeds([]string{
					"testnet-seed.bitcoin.jonasschnelli.ch",
					"seed.tbtc.petertodd.org",
					"seed.testnet.bitcoin.sprovoost.nl",
					"testnet-seed.bluematt.me",
				}, 18333)
			}
		}()
	}
}

func ClosePeerDB() {
	if PeerDB != nil {
		fmt.Println("Closing peer DB")
		PeerDB.Sync()
		PeerDB.Defrag(true)
		PeerDB.Close()
		PeerDB = nil
	}
}
