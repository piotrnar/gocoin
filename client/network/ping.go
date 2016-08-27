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
	c.X.PingHistory[c.X.PingHistoryIdx] = int(ms)
	c.X.PingHistoryIdx = (c.X.PingHistoryIdx+1)%PingHistoryLength
	c.PingInProgress = nil
	c.NextPing = time.Now().Add(PingPeriod)
	c.Mutex.Unlock()
}


// Make sure to called it within c.Mutex.Lock()
func (c *OneConnection) GetAveragePing() int {
	if c.Node.Version>60000 {
		var pgs[PingHistoryLength] int
		copy(pgs[:], c.X.PingHistory[:])
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
		if v.X.Incomming && InConsActive < atomic.LoadUint32(&common.CFG.Net.MaxInCons) {
			// If this is an incomming connection, but we are not full yet, ignore it
			continue
		}
		v.Mutex.Lock()
		ap := v.GetAveragePing()
		if ap > worst_ping && v.Node.Version<70014 {
			worst_ping = ap
			worst_conn = v
		}
		v.Mutex.Unlock()
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
