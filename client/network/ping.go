package network

import (
	"time"
	"sort"
	"sync/atomic"
	"crypto/rand"
	"github.com/piotrnar/gocoin/client/common"
)


func (c *OneConnection) HandlePong() {
	ms := time.Now().Sub(c.LastPingSent) / time.Millisecond
	if common.DebugLevel>1 {
		println(c.PeerAddr.Ip(), "pong after", ms, "ms", time.Now().Sub(c.LastPingSent).String())
	}
	c.Mutex.Lock()
	c.PingHistory[c.PingHistoryIdx] = int(ms)
	c.PingHistoryIdx = (c.PingHistoryIdx+1)%PingHistoryLength
	c.PingInProgress = nil
	c.NextPing = time.Now().Add(PingPeriod)
	c.Mutex.Unlock()
}


// Make sure to called it within c.Mutex.Lock()
func (c *OneConnection) GetAveragePing() int {
	if c.Node.Version>60000 {
		var pgs[PingHistoryLength] int
		copy(pgs[:], c.PingHistory[:])
		sort.Ints(pgs[:])
		var sum int
		for i:=0; i<PingHistoryValid; i++ {
			sum += pgs[i]
		}
		return sum/PingHistoryValid
	} else {
		return PingAssumedIfUnsupported
	}
}

// This function should be called only when OutConsActive >= MaxOutCons
func drop_slowest_peer() {
	var worst_ping int
	var worst_conn *OneConnection
	Mutex_net.Lock()
	for _, v := range OpenCons {
		if v.Incoming && InConsActive < atomic.LoadUint32(&common.CFG.Net.MaxInCons) {
			// If this is an incoming connection, but we are not full yet, ignore it
			continue
		}
		v.Mutex.Lock()
		ap := v.GetAveragePing()
		v.Mutex.Unlock()
		if ap > worst_ping {
			worst_ping = ap
			worst_conn = v
		}
	}
	if worst_conn != nil {
		if common.DebugLevel > 0 {
			println("Droping slowest peer", worst_conn.PeerAddr.Ip(), "/", worst_ping, "ms")
		}
		worst_conn.Disconnect()
		common.CountSafe("PeersDropped")
	}
	Mutex_net.Unlock()
}


func (c *OneConnection) TryPing() {
	if c.Node.Version>60000 && c.PingInProgress == nil && time.Now().After(c.NextPing) {
		/*&&len(c.Send.Buf)==0 && len(c.GetBlocksInProgress)==0*/
		c.PingInProgress = make([]byte, 8)
		rand.Read(c.PingInProgress[:])
		c.SendRawMsg("ping", c.PingInProgress)
		c.LastPingSent = time.Now()
		//println(c.PeerAddr.Ip(), "ping...")
		return
	}
}
