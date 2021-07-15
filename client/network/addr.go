package network

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/piotrnar/gocoin/client/common"
	"github.com/piotrnar/gocoin/lib/btc"
	"github.com/piotrnar/gocoin/lib/others/peersdb"
	"github.com/piotrnar/gocoin/lib/others/qdb"
	"github.com/piotrnar/gocoin/lib/others/sys"
)

var (
	ExternalIp4            map[uint32][2]uint = make(map[uint32][2]uint) // [0]-count, [1]-timestamp
	ExternalIpMutex        sync.Mutex
	ExternalIpExpireTicker int
)

func ExternalAddrLen() (res int) {
	ExternalIpMutex.Lock()
	res = len(ExternalIp4)
	ExternalIpMutex.Unlock()
	return
}

type ExternalIpRec struct {
	IP  uint32
	Cnt uint
	Tim uint
}

// GetExternalIPs returns the list sorted by "freshness".
func GetExternalIPs() (arr []ExternalIpRec) {
	ExternalIpMutex.Lock()
	defer ExternalIpMutex.Unlock()

	arr = make([]ExternalIpRec, 0, len(ExternalIp4)+1)
	var arx *ExternalIpRec

	if external_ip := common.GetExternalIp(); external_ip != "" {
		var a, b, c, d int
		if n, _ := fmt.Sscanf(external_ip, "%d.%d.%d.%d", &a, &b, &c, &d); n == 4 && (uint(a|b|c|d)&0xffffff00) == 0 {
			arx = new(ExternalIpRec)
			arx.IP = (uint32(a) << 24) | (uint32(b) << 16) | (uint32(c) << 8) | uint32(d)
			arx.Cnt = 1e6
			arx.Tim = uint(time.Now().Unix()) + 60
			arr = append(arr, *arx)
		}
	}

	if len(ExternalIp4) > 0 {
		for ip, rec := range ExternalIp4 {
			if arx != nil && arx.IP == ip {
				continue
			}
			arr = append(arr, ExternalIpRec{IP: ip, Cnt: rec[0], Tim: rec[1]})
		}

		if len(arr) > 1 {
			sort.Slice(arr, func(i, j int) bool {
				if arr[i].Cnt > 3 && arr[j].Cnt > 3 || arr[i].Cnt == arr[j].Cnt {
					return arr[i].Tim > arr[j].Tim
				}
				return arr[i].Cnt > arr[j].Cnt
			})
		}
	}

	return
}

func BestExternalAddr() []byte {
	arr := GetExternalIPs()

	// Expire any extra IP if it has been stale for more than an hour
	if len(arr) > 1 {
		worst := &arr[len(arr)-1]

		if uint(time.Now().Unix())-worst.Tim > 3600 {
			common.CountSafe("ExternalIPExpire")
			ExternalIpMutex.Lock()
			if ExternalIp4[worst.IP][0] == worst.Cnt {
				delete(ExternalIp4, worst.IP)
			}
			ExternalIpMutex.Unlock()
		}
	}

	res := make([]byte, 26)
	binary.LittleEndian.PutUint64(res[0:8], common.Services)
	// leave ip6 filled with zeros, except for the last 2 bytes:
	res[18], res[19] = 0xff, 0xff
	if len(arr) > 0 {
		binary.BigEndian.PutUint32(res[20:24], arr[0].IP)
	}
	binary.BigEndian.PutUint16(res[24:26], common.DefaultTcpPort())
	return res
}

// HandleGetaddr sends the response to "getaddr" message.
// Sends addr message with the most recent seen-alive peers from our database
func (c *OneConnection) HandleGetaddr() {
	pers := peersdb.GetRecentPeersExt(MaxAddrsPerMessage, false, func(p *peersdb.PeerAddr) bool {
		return p.Banned != 0 || !p.SeenAlive // we only return addresses that we've seen alive
	})
	if len(pers) > 0 {
		buf := new(bytes.Buffer)
		btc.WriteVlen(buf, uint64(len(pers)))
		for i := range pers {
			binary.Write(buf, binary.LittleEndian, pers[i].Time)
			buf.Write(pers[i].NetAddr.Bytes())
		}
		c.SendRawMsg("addr", buf.Bytes())
	}
}

func (c *OneConnection) SendOwnAddr() {
	if ExternalAddrLen() > 0 {
		buf := new(bytes.Buffer)
		btc.WriteVlen(buf, uint64(1))
		binary.Write(buf, binary.LittleEndian, uint32(time.Now().Unix()))
		buf.Write(BestExternalAddr())
		c.SendRawMsg("addr", buf.Bytes())
	}
}

// ParseAddr parses the network's "addr" message.
func (c *OneConnection) ParseAddr(pl []byte) {
	b := bytes.NewBuffer(pl)
	cnt, _ := btc.ReadVLen(b)
	for i := 0; i < int(cnt); i++ {
		var buf [30]byte
		n, e := b.Read(buf[:])
		if n != len(buf) || e != nil {
			common.CountSafe("AddrError")
			c.DoS("AddrError")
			break
		}
		a := peersdb.NewPeer(buf[:])
		if !sys.ValidIp4(a.Ip4[:]) {
			common.CountSafe("AddrIPinv")
		} else {
			now := uint32(time.Now().Unix())
			if a.Time > now {
				if a.Time-now >= 3600 { // It more than 1 hour in the future, reject it
					common.CountSafe("AddrFuture")
					if c.Misbehave("AdrFuture", 50) {
						break
					}
				}
				a.Time = now
			}
			k := qdb.KeyType(a.UniqID())
			v := peersdb.PeerDB.Get(k)
			if v != nil {
				op := peersdb.NewPeer(v[:])
				if a.Time > op.Time {
					op.Time = a.Time
				}
				a = op
				common.CountSafe("AddrOld")
			} else {
				if peersdb.PeerDB.Count() >= peersdb.MaxPeersInDB+peersdb.MaxPeersDeviation {
					common.CountSafe("AddrNewSkept")
					continue
				}
				common.CountSafe("AddrNewTaken")
			}
			peersdb.PeerDB.Put(k, a.Bytes())
		}
	}
}
