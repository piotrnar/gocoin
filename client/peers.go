package main

import (
	"fmt"
	"bytes"
	"time"
//	"encoding/hex"
	"encoding/binary"
	"github.com/piotrnar/qdb"
	"hash/crc64"
)

var (
	peerDB *qdb.DB
	crctab = crc64.MakeTable(crc64.ISO)
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
	
	if len(v) == 42 {
		p.Banned = binary.LittleEndian.Uint32(v[30:34])
		p.FirstSeen = binary.LittleEndian.Uint32(v[34:38])
		p.TimesSeen = binary.LittleEndian.Uint32(v[38:42])
	} else {
		p.FirstSeen = p.Time
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
	}
	return b.Bytes()
}


func (p *onePeer) String() (s string) {
	s = fmt.Sprintf("%d.%d.%d.%d:%d  0x%x, seen %d times in %s..%s",
		p.Ip4[0], p.Ip4[1], p.Ip4[2], p.Ip4[3], p.Port, p.Services, p.TimesSeen,
		time.Unix(int64(p.FirstSeen), 0).Format("06-01-02 15:04:05"),
		time.Unix(int64(p.Time), 0).Format("06-01-02 15:04:05"),)
	if p.Banned!=0 {
		s += " BAN at "+time.Unix(int64(p.Banned), 0).Format("06-01-02 15:04:05")
	}
	return
}


func (p *onePeer) QdbKey() (qdb.KeyType) {
	h := crc64.New(crctab)
	h.Write(p.Ip6[:])
	h.Write(p.Ip4[:])
	h.Write([]byte{byte(p.Port>>8),byte(p.Port)})
	return qdb.KeyType(h.Sum64())
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
		k := a.QdbKey()
		v := peerDB.Get(k)
		if v != nil {
			prv := newPeer(v[:])
			println(a.String(), "already in the db", prv.Time, a.Time)
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


func init () {
	peerDB, _ = qdb.NewDB("peers")
}
