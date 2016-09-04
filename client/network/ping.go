package network

import (
	"time"
	"sort"
	"sync/atomic"
	"crypto/rand"
	"github.com/piotrnar/gocoin/client/common"
)

const (
	PingPeriod = 15*time.Second
	PingTimeout = 10*time.Second
	PingHistoryLength = 20
	PingAssumedIfUnsupported = 4999 // ms

	DropSlowestEvery = 10*time.Minute // Look for the slowest peer and drop it
)


func (c *OneConnection) HandlePong() {
	ms := time.Now().Sub(c.LastPingSent) / time.Millisecond
	if common.DebugLevel>1 {
		println(c.PeerAddr.Ip(), "pong after", ms, "ms", time.Now().Sub(c.LastPingSent).String())
	}
	if ms==0 {
		println(c.ConnID, "Ping returned after 0ms")
		ms = 1
	}
	c.Mutex.Lock()
	c.X.PingHistory[c.X.PingHistoryIdx] = int(ms)
	c.X.PingHistoryIdx = (c.X.PingHistoryIdx+1)%PingHistoryLength
	c.PingInProgress = nil
	c.NextPing = time.Now().Add(PingPeriod)
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


type conn_list_to_drop []struct {
	c *OneConnection
	ping int
	blks uint64
}

func (l conn_list_to_drop) Len() int {
	return len(l)
}

func (l conn_list_to_drop) Less(a, b int) bool {
	if l[a].blks == l[b].blks {
		return l[a].ping > l[b].ping
	}
	return l[a].blks < l[b].blks
}

func (l conn_list_to_drop) Swap(a, b int) {
	l[a], l[b] = l[b], l[a]
}


// This function should be called only when OutConsActive >= MaxOutCons
func drop_worst_peer() {
	var list conn_list_to_drop
	var cnt int
	var any_ping bool
	Mutex_net.Lock()
	defer Mutex_net.Unlock()
	list = make(conn_list_to_drop, len(OpenCons))
	for _, v := range OpenCons {
		v.Mutex.Lock()
		list[cnt].c = v
		list[cnt].ping = v.GetAveragePing()
		list[cnt].blks = v.X.BlocksReceived
		if list[cnt].ping>0 {
			any_ping = true
		}
		v.Mutex.Unlock()
		cnt++
	}
	if !any_ping {
		return
	}
	sort.Sort(list)
	for cnt = range list {
		var drop_now bool
		if list[cnt].c.X.Incomming {
			drop_now = InConsActive+2 > atomic.LoadUint32(&common.CFG.Net.MaxInCons)
		} else {
			drop_now = OutConsActive+2> atomic.LoadUint32(&common.CFG.Net.MaxOutCons)
		}
		if drop_now {
			list[cnt].c.Disconnect()
			return
		}
	}
}


func (c *OneConnection) TryPing() bool {
	if c.Node.Version>60000 && c.PingInProgress == nil && time.Now().After(c.NextPing) {
		c.PingInProgress = make([]byte, 8)
		rand.Read(c.PingInProgress[:])
		c.SendRawMsg("ping", c.PingInProgress)
		c.LastPingSent = time.Now()
		//println(c.PeerAddr.Ip(), "ping...")
		return true
	}
	return false
}
