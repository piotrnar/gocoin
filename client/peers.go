package main

import (
	"os"
	"fmt"
	"net"
	"time"
	"bytes"
	"strings"
	"encoding/binary"
	"github.com/piotrnar/qdb"
	"hash/crc64"
	"github.com/piotrnar/gocoin/btc"
)

const (
	defragEvery = (60*time.Second) // Once a minute should be more than enough
	expirePeerAfter = 3*3600 // 3 hours - https://en.bitcoin.it/wiki/Protocol_specification#addr
)

var (
	peerDB *qdb.DB
	crctab = crc64.MakeTable(crc64.ISO)
	
	proxyPeer *onePeer // when this is not nil we should only connect to this single node

	nextDefrag time.Time
)

type onePeer struct {
	Services uint64
	Ip6 [12]byte
	Ip4 [4]byte
	Port uint16
	
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


func newIncommingPeer(ipstr string) (p *onePeer) {
	x := strings.Index(ipstr, ":")
	if x != -1 {
		ipstr = ipstr[:x] // remove port number
	}
	ip := net.ParseIP(ipstr)
	if ip != nil && len(ip)==16 {
		p = new(onePeer)
		p.Time = uint32(time.Now().Unix())
		p.Services = 1
		copy(p.Ip6[:], ip[:12])
		copy(p.Ip4[:], ip[12:16])
		p.Port = DefaultTcpPort
		p.Save()
	} else {
		println("newIncommingPeer: error parsing IP", ipstr)
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


func pers_do_cleanup() {
	if time.Now().After(nextDefrag) {
		var delcnt uint32
		now := uint32(time.Now().Unix())
		todel := make([]qdb.KeyType, peerDB.Count())
		peerDB.Browse(func(k qdb.KeyType, v []byte) bool {
			if int(now - newPeer(v).Time) > expirePeerAfter {
				todel[delcnt] = k // we cannot call Del() from here
				delcnt++
			}
			return true
		})
		for delcnt > 0 {
			delcnt--
			peerDB.Del(todel[delcnt])
		}
		peerDB.Defrag()
		nextDefrag = time.Now().Add(defragEvery)
	}
}


func (p *onePeer) Save() {
	peerDB.Put(qdb.KeyType(p.UniqID()), p.Bytes())
	pers_do_cleanup()
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
	p.Time = uint32(time.Now().Unix()) - 600 // make it 10 min older
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


func ParseAddr(pl []byte) {
	b := bytes.NewBuffer(pl)
	now := uint32(time.Now().Unix())
	cnt, _ := btc.ReadVLen(b)
	for i := 0; i < int(cnt); i++ {
		var buf [30]byte
		n, e := b.Read(buf[:])
		if n!=len(buf) || e!=nil {
			println("ParseAddr:", n, e)
			break
		}
		a := newPeer(buf[:])
		if a.Time > now - expirePeerAfter {
			k := qdb.KeyType(a.UniqID())
			v := peerDB.Get(k)
			if v != nil {
				a.Banned = newPeer(v[:]).Banned
			}
			peerDB.Put(k, a.Bytes())
		}
	}
	peerDB.Defrag()
}


func getBestPeer() (p *onePeer) {
	if proxyPeer!=nil {
		if !connectionActive(proxyPeer) {
			p = proxyPeer
		}
		return
	}
	
	var best_time uint32
	peerDB.Browse(func(k qdb.KeyType, v []byte) bool {
		ad := newPeer(v)
		if ad.Banned!=0 && ad.Ip4!=[4]byte{127,0,0,1} {
			if ad.Time > best_time && !connectionActive(ad) {
				best_time = ad.Time
				p = ad
			}
		}
		return true
	})

	return 
}


func initSeeds(seeds []string, port int) {
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
					p.Port = uint16(port)
					p.Save()
				}
			}
		}
	}
}


func initPeers(dir string) {
	nextDefrag = time.Now().Add(defragEvery)

	peerDB, _ = qdb.NewDB(dir+"peers")
	if peerDB.Count()==0 {
		if !*testnet {
			initSeeds([]string{"seed.bitcoin.sipa.be", "dnsseed.bluematt.me",
				"dnsseed.bitcoin.dashjr.org", "bitseed.xf2.org"}, 8333)
		} else {
			initSeeds([]string{"testnet-seed.bitcoin.petertodd.org","testnet-seed.bluematt.me"}, 18333)
		}
		println("peerDB initiated with ", peerDB.Count(), "seeds")
	}

	if *proxy != "" {
		oa, e := net.ResolveTCPAddr("tcp4", *proxy)
		if e != nil {
			if strings.HasPrefix(e.Error(), "missing port in address") {
				oa, e = net.ResolveTCPAddr("tcp4", fmt.Sprint(*proxy,":",DefaultTcpPort))
			}
			if e!=nil {
				println(e.Error())
				os.Exit(1)
			}
		}
		proxyPeer = new(onePeer)
		proxyPeer.Services = 1
		copy(proxyPeer.Ip4[:], oa.IP[0:4])
		proxyPeer.Port = uint16(oa.Port)
	}
	
}


func show_addresses(par string) {
	fmt.Println(peerDB.Count(), "peers in the database:")
	cnt := 0
	peerDB.Browse(func(k qdb.KeyType, v []byte) bool {
		cnt++
		fmt.Printf("%4d) %s\n", cnt, newPeer(v).String())
		return true
	})
}


func init() {
	newUi("pers", false, show_addresses, "Dump pers database (warning: may be long)")
}
