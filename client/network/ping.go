package network

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"os"
	"sort"
	"time"

	"github.com/piotrnar/gocoin/client/common"
)

const (
	PingHistoryLength        = 20
	PingAssumedIfUnsupported = 4999 // ms
	PingAssumedIfUnknown     = 9999 // ms
)

func (c *OneConnection) HandlePong(pl []byte) {
	if pl != nil {
		if !bytes.Equal(pl, c.PingInProgress) {
			common.CountSafe("PongMismatch")
			return
		}
		common.CountSafe("PongOK")
		c.ExpireHeadersAndGetData(nil, c.X.PingSentCnt)
	} else {
		common.CountSafe("PongTimeout")
	}
	ms := time.Now().Sub(c.LastPingSent) / time.Millisecond
	if ms == 0 {
		//println(c.ConnID, "Ping returned after 0ms")
		ms = 1
	}
	c.Mutex.Lock()
	c.X.PingHistory[c.X.PingHistoryIdx] = int(ms)
	c.X.PingHistoryIdx = (c.X.PingHistoryIdx + 1) % PingHistoryLength
	c.PingInProgress = nil
	c.Mutex.Unlock()
}

// GetAveragePing returns the (median) average ping.
// Make sure to call it within c.Mutex.Lock().
func (c *OneConnection) GetAveragePing() int {
	if !c.X.VersionReceived {
		return 0
	}
	if c.Node.Version > 60000 {
		var pgs [PingHistoryLength]int
		var act_len int
		for _, p := range c.X.PingHistory {
			if p != 0 {
				pgs[act_len] = p
				act_len++
			}
		}
		if act_len == 0 {
			return PingAssumedIfUnknown
		}
		sort.Ints(pgs[:act_len])
		return pgs[act_len/2]
	} else {
		return PingAssumedIfUnsupported
	}
}

type SortedConnections []struct {
	Conn          *OneConnection
	Ping          int
	BlockCount    int
	TxsCount      int
	MinutesOnline int
	Special       bool
}

// GetSortedConnections returns the slowest peers first.
// Make sure to call it with locked Mutex_net.
func GetSortedConnections() (list SortedConnections, any_ping bool) {
	var cnt int
	var now time.Time
	var tlist SortedConnections
	now = time.Now()
	tlist = make(SortedConnections, len(OpenCons))
	for _, v := range OpenCons {
		v.Mutex.Lock()
		tlist[cnt].Conn = v
		tlist[cnt].Ping = v.GetAveragePing()
		tlist[cnt].BlockCount = len(v.blocksreceived)
		tlist[cnt].TxsCount = v.X.TxsReceived
		tlist[cnt].Special = v.X.IsSpecial || v.X.Authorized
		if v.X.VersionReceived == false || v.X.ConnectedAt.IsZero() {
			tlist[cnt].MinutesOnline = 0
		} else {
			tlist[cnt].MinutesOnline = int(now.Sub(v.X.ConnectedAt) / time.Minute)
		}
		v.Mutex.Unlock()

		if tlist[cnt].Ping > 0 {
			any_ping = true
		}

		cnt++
	}
	if cnt > 0 {
		list = make(SortedConnections, len(tlist))
		var ignore_bcnt bool // otherwise count blocks
		var idx, best_idx, bcnt, best_bcnt, best_tcnt, best_ping int

		for idx = len(list) - 1; idx >= 0; idx-- {
			best_idx = -1
			for i, v := range tlist {
				if v.Conn == nil {
					continue
				}
				if best_idx < 0 {
					best_idx = i
					best_tcnt = v.TxsCount
					best_bcnt = v.BlockCount
					best_ping = v.Ping
				} else {
					if ignore_bcnt {
						bcnt = best_bcnt
					} else {
						bcnt = v.BlockCount
					}
					if best_bcnt < bcnt ||
						best_bcnt == bcnt && best_tcnt < v.TxsCount ||
						best_bcnt == bcnt && best_tcnt == v.TxsCount && best_ping > v.Ping {
						best_bcnt = v.BlockCount
						best_tcnt = v.TxsCount
						best_ping = v.Ping
						best_idx = i
					}
				}
			}
			list[idx] = tlist[best_idx]
			tlist[best_idx].Conn = nil
			ignore_bcnt = !ignore_bcnt
		}
	}
	return
}

// drop_worst_peer should be called only when OutConsActive >= MaxOutCons.
func drop_worst_peer() bool {
	var list SortedConnections
	var any_ping bool

	Mutex_net.Lock()
	defer Mutex_net.Unlock()

	list, any_ping = GetSortedConnections()
	if !any_ping { // if "list" is empty "any_ping" will also be false
		return false
	}

	for _, v := range list {
		if v.MinutesOnline < OnlineImmunityMinutes {
			continue
		}
		if v.Special {
			continue
		}
		if v.Conn.X.Incomming {
			if InConsActive+2 > common.GetUint32(&common.CFG.Net.MaxInCons) {
				common.CountSafe("PeerInDropped")
				if common.FLAG.Log {
					f, _ := os.OpenFile("drop_log.txt", os.O_CREATE|os.O_RDWR|os.O_APPEND, 0660)
					if f != nil {
						fmt.Fprintf(f, "%s: Drop incoming id:%d  blks:%d  txs:%d  ping:%d  mins:%d\n",
							time.Now().Format("2006-01-02 15:04:05"),
							v.Conn.ConnID, v.BlockCount, v.TxsCount, v.Ping, v.MinutesOnline)
						f.Close()
					}
				}
				v.Conn.Disconnect(true, "PeerInDropped")
				return true
			}
		} else {
			if OutConsActive+2 > common.GetUint32(&common.CFG.Net.MaxOutCons) {
				common.CountSafe("PeerOutDropped")
				if common.FLAG.Log {
					f, _ := os.OpenFile("drop_log.txt", os.O_CREATE|os.O_RDWR|os.O_APPEND, 0660)
					if f != nil {
						fmt.Fprintf(f, "%s: Drop outgoing id:%d  blks:%d  txs:%d  ping:%d  mins:%d\n",
							time.Now().Format("2006-01-02 15:04:05"),
							v.Conn.ConnID, v.BlockCount, v.TxsCount, v.Ping, v.MinutesOnline)
						f.Close()
					}
				}
				v.Conn.Disconnect(true, "PeerOutDropped")
				return true
			}
		}
	}
	return false
}

func (c *OneConnection) TryPing(now time.Time) bool {
	if c.Node.Version <= 60000 {
		return false // insufficient protocol version
	}

	pingdur := common.GetDuration(&common.PingPeerEvery)
	if pingdur == 0 {
		return false // pinging disabled in global config
	}

	if now.Sub(c.LastPingSent) < pingdur {
		return false // not yet...
	}

	if c.PingInProgress != nil {
		c.HandlePong(nil) // this will set PingInProgress to nil
	}

	c.X.PingSentCnt++
	c.PingInProgress = make([]byte, 8)
	rand.Read(c.PingInProgress[:])
	c.SendRawMsg("ping", c.PingInProgress)
	c.LastPingSent = time.Now()
	//println(c.PeerAddr.Ip(), "ping...")
	return true
}
