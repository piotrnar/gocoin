package network

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"github.com/piotrnar/gocoin/client/common"
	"github.com/piotrnar/gocoin/lib/btc"
	"github.com/piotrnar/gocoin/lib/others/peersdb"
	"github.com/piotrnar/gocoin/lib/others/qdb"
	"github.com/piotrnar/gocoin/lib/others/sys"
	"sort"
	"sync"
	"time"
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

// Returns the list sorted by "freshness"
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
			if arx != nil && arx.IP==ip {
				continue
			}
			arr = append(arr, ExternalIpRec{IP:ip, Cnt:rec[0], Tim:rec[1]})
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

func (c *OneConnection) SendAddr() {
	pers := peersdb.GetBestPeers(MaxAddrsPerMessage, nil)
	maxtime := uint32(time.Now().Unix() + 3600)
	if len(pers) > 0 {
		buf := new(bytes.Buffer)
		btc.WriteVlen(buf, uint64(len(pers)))
		for i := range pers {
			if pers[i].Time > maxtime {
				println("addr", i, "time in future", pers[i].Time, maxtime, "should not happen")
				pers[i].Time = maxtime - 7200
			}
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

// Parese network's "addr" message
func (c *OneConnection) ParseAddr(pl []byte) {
	b := bytes.NewBuffer(pl)
	cnt, _ := btc.ReadVLen(b)
	for i := 0; i < int(cnt); i++ {
		var buf [30]byte
		n, e := b.Read(buf[:])
		if n != len(buf) || e != nil {
			common.CountSafe("AddrError")
			c.DoS("AddrError")
			//println("ParseAddr:", n, e)
			break
		}
		a := peersdb.NewPeer(buf[:])
		if !sys.ValidIp4(a.Ip4[:]) {
			common.CountSafe("AddrInvalid")
			/*if c.Misbehave("AddrLocal", 1) {
				break
			}*/
			//print(c.PeerAddr.Ip(), " ", c.Node.Agent, " ", c.Node.Version, " addr local ", a.String(), "\n> ")
		} else if time.Unix(int64(a.Time), 0).Before(time.Now().Add(time.Hour)) {
			if time.Now().Before(time.Unix(int64(a.Time), 0).Add(peersdb.ExpirePeerAfter)) {
				k := qdb.KeyType(a.UniqID())
				v := peersdb.PeerDB.Get(k)
				if v != nil {
					a.Banned = peersdb.NewPeer(v[:]).Banned
				}
				a.Time = uint32(time.Now().Add(-5 * time.Minute).Unix()) // add new peers as not just alive
				if a.Time > uint32(time.Now().Unix()) {
					println("wtf", a.Time, time.Now().Unix())
				}
				peersdb.PeerDB.Put(k, a.Bytes())
			} else {
				common.CountSafe("AddrStale")
			}
		} else {
			if c.Misbehave("AddrFuture", 50) {
				break
			}
		}
	}
}
