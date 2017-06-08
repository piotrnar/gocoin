package network

import (
	"os"
	"fmt"
	"time"
	"sort"
	"sync/atomic"
	"crypto/rand"
	"github.com/piotrnar/gocoin/client/common"
)

const (
	PingHistoryLength = 20
	PingAssumedIfUnsupported = 4999 // ms
)


func (c *OneConnection) HandlePong() {
	ms := time.Now().Sub(c.LastPingSent) / time.Millisecond
	if common.DebugLevel>1 {
		println(c.PeerAddr.Ip(), "pong after", ms, "ms", time.Now().Sub(c.LastPingSent).String())
	}
	if ms==0 {
		//println(c.ConnID, "Ping returned after 0ms")
		ms = 1
	}
	c.Mutex.Lock()
	c.X.PingHistory[c.X.PingHistoryIdx] = int(ms)
	c.X.PingHistoryIdx = (c.X.PingHistoryIdx+1)%PingHistoryLength
	c.PingInProgress = nil
	c.Mutex.Unlock()
}


// Returns (median) average ping
// Make sure to called it within c.Mutex.Lock()
func (c *OneConnection) GetAveragePing() int {
	if c.Node.Version>60000 {
		var pgs[PingHistoryLength] int
		var act_len int
		for _, p := range c.X.PingHistory {
			if p!=0 {
				pgs[act_len] = p
				act_len++
			}
		}
		if act_len==0 {
			return 0
		}
		sort.Ints(pgs[:act_len])
		return pgs[act_len/2]
	} else {
		return PingAssumedIfUnsupported
	}
}


type SortedConnections []struct {
	Conn *OneConnection
	Ping int
	BlockCount int
	TxsCount int
	MinutesOnline int
}

func (l SortedConnections) Len() int {
	return len(l)
}

func (l SortedConnections) Less(a, b int) bool {
	// If any of the two is connected for less than one hour, just compare the ping
	if l[a].MinutesOnline<60 || l[b].MinutesOnline<60 {
		return l[a].Ping > l[b].Ping
	}

	if l[a].BlockCount == l[b].BlockCount {
		if l[a].TxsCount == l[b].TxsCount {
			return l[a].Ping > l[b].Ping
		}
		return l[a].TxsCount < l[b].TxsCount
	}
	return l[a].BlockCount < l[b].BlockCount
}

func (l SortedConnections) Swap(a, b int) {
	l[a], l[b] = l[b], l[a]
}

// Make suure to call it with locked Mutex_net
func GetSortedConnections(all bool) (list SortedConnections, any_ping bool) {
	var cnt int
	var now time.Time
	var time_online time.Duration
	now = time.Now()
	list = make(SortedConnections, len(OpenCons))
	for _, v := range OpenCons {
		v.Mutex.Lock()
		// do not drop peers that connected just recently
		time_online = now.Sub(v.X.ConnectedAt)
		if all || time_online >= common.DropSlowestEvery {
			list[cnt].Conn = v
			list[cnt].Ping = v.GetAveragePing()
			list[cnt].BlockCount = len(v.blocksreceived)
			list[cnt].TxsCount = v.X.TxsReceived
			list[cnt].MinutesOnline = int(time_online/time.Minute)
			if list[cnt].Ping>0 {
				any_ping = true
			}
			cnt++
		}
		v.Mutex.Unlock()
	}
	if cnt > 0 {
		list = list[:cnt]
		sort.Sort(list)
	} else {
		list = nil
	}
	return
}

// This function should be called only when OutConsActive >= MaxOutCons
func drop_worst_peer() bool {
	var list SortedConnections
	var any_ping bool

	Mutex_net.Lock()
	defer Mutex_net.Unlock()

	list, any_ping = GetSortedConnections(false)
	if !any_ping { // if "list" is empty "any_ping" will also be false
		return false
	}

	for _, v := range list {
		if v.Conn.X.Incomming {
			if InConsActive+2 > atomic.LoadUint32(&common.CFG.Net.MaxInCons) {
				common.CountSafe("PeerInDropped")
				f, _ := os.OpenFile("drop_log.txt", os.O_CREATE|os.O_RDWR|os.O_APPEND, 0660);
				if f!=nil {
					fmt.Fprintf(f, "%s: Drop incomming id:%d  blks:%d  txs:%d  ping:%d  mins:%d\n",
						time.Now().Format("2006-01-02 15:04:05"),
						v.Conn.ConnID, v.BlockCount, v.TxsCount, v.Ping, v.MinutesOnline)
					f.Close()
				}
				v.Conn.Disconnect()
				return true
			}
		} else {
			if OutConsActive+2 > atomic.LoadUint32(&common.CFG.Net.MaxOutCons) {
				common.CountSafe("PeerOutDropped")
				f, _ := os.OpenFile("drop_log.txt", os.O_CREATE|os.O_RDWR|os.O_APPEND, 0660);
				if f!=nil {
					fmt.Fprintf(f, "%s: Drop outgoing id:%d  blks:%d  txs:%d  ping:%d  mins:%d\n",
						time.Now().Format("2006-01-02 15:04:05"),
						v.Conn.ConnID, v.BlockCount, v.TxsCount, v.Ping, v.MinutesOnline)
					f.Close()
				}
				v.Conn.Disconnect()
				return true
			}
		}
	}
	return false
}


func (c *OneConnection) TryPing() bool {
	if common.PingPeerEvery==0 {
		return false // pinging disabled in global config
	}

	if c.Node.Version<=60000 {
		return false // insufficient protocol version
	}

	if time.Now().Before(c.LastPingSent.Add(common.PingPeerEvery)) {
		return false // not yet...
	}

	if c.PingInProgress!=nil {
		if common.DebugLevel > 0 {
			println(c.PeerAddr.Ip(), "ping timeout")
		}
		common.CountSafe("PingTimeout")
		c.HandlePong()  // this will set PingInProgress to nil
	}

	c.PingInProgress = make([]byte, 8)
	rand.Read(c.PingInProgress[:])
	c.SendRawMsg("ping", c.PingInProgress)
	c.LastPingSent = time.Now()
	//println(c.PeerAddr.Ip(), "ping...")
	return true
}
