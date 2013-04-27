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
)

const defragEvery = (10*time.Second)

var (
	peerDB *qdb.DB
	crctab = crc64.MakeTable(crc64.ISO)
	
	proxyPeer *onePeer // when this is not nil we should only connect to this single node

	nextDefrag time.Time
)

type onePeer struct {
	Time uint32  // When seen last time
	Services uint64
	Ip6 [12]byte
	Ip4 [4]byte
	Port uint16
	
	Banned uint32 // time when this address baned or zero if never
	FirstSeen uint32 // time when this address was seen for the first time
	TimesSeen uint32 // how many times this address have been seen
	
	FailedLast uint32
	FailedCount uint32
	
	ConnectedLast uint32
	ConnectedCount uint32

	BytesReceived uint64
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
	
	if len(v) >= 42 {
		p.Banned = binary.LittleEndian.Uint32(v[30:34])
		p.FirstSeen = binary.LittleEndian.Uint32(v[34:38])
		p.TimesSeen = binary.LittleEndian.Uint32(v[38:42])
	} else {
		p.FirstSeen = p.Time
	}

	if len(v) >= 58 {
		p.FailedLast = binary.LittleEndian.Uint32(v[42:46])
		p.FailedCount = binary.LittleEndian.Uint32(v[46:50])
		p.ConnectedLast = binary.LittleEndian.Uint32(v[50:54])
		p.ConnectedCount = binary.LittleEndian.Uint32(v[54:58])
	}
	
	if len(v) >= 66 {
		p.BytesReceived = binary.LittleEndian.Uint64(v[58:66])
	}
	return
}


func (p *onePeer) Bytes(all bool) []byte {
	b := new(bytes.Buffer)
	binary.Write(b, binary.LittleEndian, p.Time)
	binary.Write(b, binary.LittleEndian, p.Services)
	b.Write(p.Ip6[:])
	b.Write(p.Ip4[:])
	binary.Write(b, binary.BigEndian, p.Port)
	if all {
		binary.Write(b, binary.LittleEndian, p.Banned)
		binary.Write(b, binary.LittleEndian, p.FirstSeen)
		binary.Write(b, binary.LittleEndian, p.TimesSeen)
		
		binary.Write(b, binary.LittleEndian, p.FailedLast)
		binary.Write(b, binary.LittleEndian, p.FailedCount)
		binary.Write(b, binary.LittleEndian, p.ConnectedLast)
		binary.Write(b, binary.LittleEndian, p.ConnectedCount)
		
		binary.Write(b, binary.LittleEndian, p.BytesReceived)
	}
	return b.Bytes()
}


func (p *onePeer) Save() {
	peerDB.Put(qdb.KeyType(p.UniqID()), p.Bytes(true))
	if nextDefrag.Before(time.Now()) {
		peerDB.Defrag()
		nextDefrag = time.Now().Add(defragEvery)
	}
}


func (p *onePeer) Failed() {
	p.FailedCount++
	p.FailedLast = uint32(time.Now().Unix())
	p.Save()
}


func (p *onePeer) Connected() {
	p.ConnectedCount++
	p.ConnectedLast = uint32(time.Now().Unix())
	p.Save()
}


func (p *onePeer) GotData(l int) {
	p.BytesReceived += uint64(l)
	p.Save()
}



func (p *onePeer) Ip() (string) {
	return fmt.Sprintf("%d.%d.%d.%d:%d", p.Ip4[0], p.Ip4[1], p.Ip4[2], p.Ip4[3], p.Port)
}

func (p *onePeer) String() (s string) {
	s = p.Ip()
	if p.Services!=1 {
		s += fmt.Sprintf("  serv:%x", p.Services)
	}
	if p.BytesReceived > 0 {
		s += fmt.Sprintf("  bytes:%d", p.BytesReceived)
	}
	if p.ConnectedCount > 0 {
		s += fmt.Sprintf("  connected %d times, last %s", p.ConnectedCount,
			time.Unix(int64(p.ConnectedLast), 0).Format("06-01-02 15:04:05"))
	}
	if p.FailedCount > 0 {
		s += fmt.Sprintf("  failed %d times, last %s", p.FailedCount,
			time.Unix(int64(p.FailedLast), 0).Format("06-01-02 15:04:05"))
	}
			/*
	s = fmt.Sprintf("%d.%d.%d.%d:%d  0x%x, seen %d times in %s..%s",
		p.Ip4[0], p.Ip4[1], p.Ip4[2], p.Ip4[3], p.Port, p.Services, p.TimesSeen,
		time.Unix(int64(p.FirstSeen), 0).Format("06-01-02 15:04:05"),
		time.Unix(int64(p.Time), 0).Format("06-01-02 15:04:05"),)
	*/
	if p.Banned!=0 {
		s += " BAN at "+time.Unix(int64(p.Banned), 0).Format("06-01-02 15:04:05")
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
	cnt := GetVarLen(b)
	for i := 0; i < int(cnt); i++ {
		var buf [30]byte
		n, e := b.Read(buf[:])
		if n!=len(buf) || e!=nil {
			println("ParseAddr:", n, e)
			break
		}
		a := newPeer(buf[:])
		k := qdb.KeyType(a.UniqID())
		v := peerDB.Get(k)
		if v != nil {
			prv := newPeer(v[:])
			//println(a.String(), "already in the db", prv.Time, a.Time)
			a.Banned = prv.Banned
			a.FirstSeen = prv.FirstSeen
			a.TimesSeen = a.TimesSeen+1
		}
		peerDB.Put(k, a.Bytes(true))
	}
	peerDB.Defrag()
}


func show_addresses() {
	println(peerDB.Count(), "peers in the database:")
	peerDB.Browse(func(k qdb.KeyType, v []byte) bool {
		println(" *", newPeer(v).String())
		return true
	})
}


func getBestPeer() (p *onePeer) {
	if proxyPeer!=nil {
		if !connectionActive(proxyPeer) {
			p = proxyPeer
		}
		return
	}
	
	oldest_failed := uint32(0xffffffff)
	var best_time uint32
	peerDB.Browse(func(k qdb.KeyType, v []byte) bool {
		ad := newPeer(v)
		if (ad.FailedLast < oldest_failed || 
			(ad.FailedLast == oldest_failed && ad.Time > best_time) ) &&
			!connectionActive(ad) {
			oldest_failed = ad.FailedLast
			best_time = ad.Time
			p = ad
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
				if *testnet {
					oa, e = net.ResolveTCPAddr("tcp4", *proxy+":18333")
				} else {
					oa, e = net.ResolveTCPAddr("tcp4", *proxy+":8333")
				}
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

