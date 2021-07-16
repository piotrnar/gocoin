package peersdb

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"hash/crc64"
	"io"
	"net"
	"os"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/piotrnar/gocoin/client/common"
	"github.com/piotrnar/gocoin/lib/btc"
	"github.com/piotrnar/gocoin/lib/others/qdb"
	"github.com/piotrnar/gocoin/lib/others/sys"
)

const (
	ExpireAlivePeerAfter  = (24 * time.Hour) // https://en.bitcoin.it/wiki/Protocol_specification#addr
	ExpireBannedPeerAfter = (7 * 24 * time.Hour)
	MinPeersInDB          = 5000
	MaxPeersInDB          = 70000
	MaxPeersDeviation     = 2500
	ExpirePeersPeriod     = (5 * time.Minute)
)

/*
Serialized peer record (all values are LSB unless specified otherwise):
 [0:4] - Unix timestamp of when last the peer was seen
 [4:12] - Services
 [12:24] - IPv6 (network order)
 [24:28] - IPv4 (network order)
 [28:30] - TCP port (big endian)
 [30:34] - OPTIONAL:
 	highest bit: set to 1 of peer has been seen "alive"
	low 31 bits: if present, unix timestamp of when the peer was banned divided by 2
 [35] - OPTIONAL flags
    bit(0) - Indicates BanReadon present (byte_len followed by the string)
    bit(1) - Indicates CameFromIP present (for IP4: len byte 4 followed by 4 bytes of IP)
	bits(2-7) - reserved

  Extra fields are always present int the order defined by the flags (from bit 0 to 7).
  Each extra field is one byte of length followed by the length bytes of data.

*/

var (
	PeerDB       *qdb.DB
	proxyPeer    *PeerAddr // when this is not nil we should only connect to this single node
	peerdb_mutex sync.Mutex

	Testnet     bool
	ConnectOnly string
	Services    uint64 = 1

	crctab = crc64.MakeTable(crc64.ISO)
)

type PeerAddr struct {
	btc.NetAddr
	Time       uint32 // When seen last time
	Banned     uint32 // time when this address baned or zero if never
	SeenAlive  bool
	BanReason  string
	CameFromIP []byte
	key_set    bool
	key_val    uint64

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
func get_extra_field(b *bytes.Buffer) []byte {
	le, er := b.ReadByte()
	if er != nil {
		return nil
	}
	dat := make([]byte, int(le))
	_, er = io.ReadFull(b, dat)
	if er != nil {
		return nil
	}
	return dat
}

func NewPeer(v []byte) (p *PeerAddr) {
	p = new(PeerAddr)
	if v == nil || len(v) < 30 {
		p.Time = uint32(time.Now().Unix())
		return
	}
	p.Time = binary.LittleEndian.Uint32(v[0:4])
	p.Services = binary.LittleEndian.Uint64(v[4:12])
	copy(p.Ip6[:], v[12:24])
	copy(p.Ip4[:], v[24:28])
	p.Port = binary.BigEndian.Uint16(v[28:30])
	if len(v) >= 34 {
		xd := binary.LittleEndian.Uint32(v[30:34])
		p.SeenAlive = (xd & 0x80000000) != 0
		p.Banned = (xd & 0x7fffffff) << 1
		if !p.SeenAlive && p.Banned > 1893452400 /*Year 2030*/ {
			// Convert from the old DB - TODO: remove it at some point (now is 14th of July 2021)
			p.Banned >>= 1
		}
		if len(v) >= 35 {
			extra_fields := v[34]
			if extra_fields != 0 {
				buf := bytes.NewBuffer(v[35:])
				for bit := 0; bit < 8; bit++ {
					if (extra_fields & 0x01) != 0 {
						dat := get_extra_field(buf)
						if dat == nil {
							break // error
						}
						switch bit {
						case 0:
							p.BanReason = string(dat)
						case 1:
							p.CameFromIP = dat
						}
					}
					extra_fields >>= 1
					if extra_fields == 0 {
						break
					}
				}
			}
		}
	}
	return
}

func (p *PeerAddr) Bytes() (res []byte) {
	var x_flags byte
	if p.Banned != 0 && p.BanReason != "" {
		x_flags |= 0x01
	}
	if p.CameFromIP != nil {
		x_flags |= 0x02
	}
	b := new(bytes.Buffer)
	binary.Write(b, binary.LittleEndian, p.Time)
	binary.Write(b, binary.LittleEndian, p.Services)
	b.Write(p.Ip6[:])
	b.Write(p.Ip4[:])
	binary.Write(b, binary.BigEndian, p.Port)
	if p.SeenAlive || x_flags != 0 {
		xd := p.Banned >> 1
		if p.SeenAlive {
			xd |= 0x80000000
		}
		binary.Write(b, binary.LittleEndian, xd)
	}
	if x_flags != 0 {
		b.WriteByte(x_flags)
	}
	if (x_flags & 0x01) != 0 {
		b.WriteByte(byte(len(p.BanReason)))
		b.Write([]byte(p.BanReason))
	}
	if (x_flags & 0x02) != 0 {
		b.WriteByte(byte(len(p.CameFromIP)))
		b.Write([]byte(p.CameFromIP))
	}
	res = b.Bytes()
	return
}

func (p *PeerAddr) UniqID() uint64 {
	if !p.key_set {
		h := crc64.New(crctab)
		h.Write(p.Ip6[:])
		h.Write(p.Ip4[:])
		h.Write([]byte{byte(p.Port >> 8), byte(p.Port)})
		p.key_set = true
		p.key_val = h.Sum64()
	}
	return p.key_val
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
			p = NewPeer(nil)
			p.Services = Services
			p.Port = port
			if len(ipa.IP) == 4 {
				copy(p.Ip4[:], ipa.IP[:])
			} else if len(ipa.IP) == 16 {
				copy(p.Ip4[:], ipa.IP[12:16])
				copy(p.Ip6[:], ipa.IP[:12])
			}
			if dbp := PeerDB.Get(qdb.KeyType(p.UniqID())); dbp != nil {
				p = NewPeer(dbp) // if we already had it, take the previous record
			}
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
		if p.Banned != 0 {
			println("update A time for banned peer", p.Ip())
		}
		p.Time = uint32(time.Now().Unix())
		p.Save()
	}
	return
}

func DeleteFromIP(ip []byte) int {
	var ks []qdb.KeyType
	peerdb_mutex.Lock()
	PeerDB.Browse(func(k qdb.KeyType, v []byte) uint32 {
		p := NewPeer(v)
		if p.CameFromIP != nil && bytes.Equal(ip, p.CameFromIP) {
			ks = append(ks, k)
		}
		return 0
	})
	for _, k := range ks {
		PeerDB.Del(k)
	}
	peerdb_mutex.Unlock()
	return len(ks)
}

func ExpirePeers() {
	peerdb_mutex.Lock()
	defer peerdb_mutex.Unlock()
	if PeerDB.Count() > MaxPeersInDB {
		common.CountSafe("PeersExpireNeeded")
		now := time.Now()
		expire_alive_before_time := uint32(now.Add(-ExpireAlivePeerAfter).Unix())
		expire_banned_before_time := uint32(now.Add(-ExpireBannedPeerAfter).Unix())
		recs := make(manyPeers, PeerDB.Count())
		var i, c_dead, c_seen_alive, c_banned int
		PeerDB.Browse(func(k qdb.KeyType, v []byte) uint32 {
			recs[i] = NewPeer(v)
			i++
			return 0
		})
		sort.Sort(recs)
		for i = len(recs) - 1; i > MinPeersInDB; i-- {
			var delit bool
			rec := recs[i]
			if !rec.SeenAlive {
				delit = true
				c_dead++
			} else if rec.Time < expire_alive_before_time {
				if rec.Banned == 0 {
					delit = true
					c_seen_alive++
				} else if rec.Banned < expire_banned_before_time {
					delit = true
					c_banned++
				}
			}
			if delit {
				PeerDB.Del(qdb.KeyType(rec.UniqID()))
				if PeerDB.Count() <= MaxPeersInDB-MaxPeersDeviation {
					break
				}
			}
		}
		common.CounterMutex.Lock()
		common.Counter["PeersExpiredDead"] += uint64(c_dead)
		common.Counter["PeersExpiredAlive"] += uint64(c_seen_alive)
		common.Counter["PeersExpiredBanned"] += uint64(c_banned)
		common.CounterMutex.Unlock()
		PeerDB.Defrag(false)
	} else {
		common.CountSafe("PeersExpireNone")
	}
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

	if p.CameFromIP != nil {
		if len(p.CameFromIP) == 4 {
			s += fmt.Sprintf(" from %d.%d.%d.%d", p.CameFromIP[0], p.CameFromIP[1], p.CameFromIP[2], p.CameFromIP[3])
		} else {
			s += " from " + hex.EncodeToString(p.CameFromIP)
		}
	}
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

// GetRecentPeersExt fetches a given number of best (most recenty seen) peers.
func GetRecentPeers(limit uint, sort_result bool, ignorePeer func(*PeerAddr) bool) (res manyPeers) {
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
					p := NewPeer(nil)
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
		proxyPeer = NewPeer(nil)
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
