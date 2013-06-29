package main

import (
	"time"
	"sort"
	"crypto/rand"
)


func (c *oneConnection) HandlePong() {
	ms := time.Now().Sub(c.LastPingSent) / time.Millisecond
	if dbg>1 {
		println(c.PeerAddr.Ip(), "pong after", ms, "ms", time.Now().Sub(c.LastPingSent).String())
	}
	c.PingHistory[c.PingHistoryIdx] = int(ms)
	c.PingHistoryIdx = (c.PingHistoryIdx+1)%PingHistoryLength
	c.PingInProgress = nil
	c.NextPing = time.Now().Add(PingPeriod)
}


func (c *oneConnection) GetAveragePing() int {
	if c.node.version>60000 {
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
	var worst_conn *oneConnection
	mutex.Lock()
	for _, v := range openCons {
		if v.Incomming && InConsActive < MaxInCons {
			// If this is an incomming connection, but we are not full yet, ignore it
			continue
		}
		ap := v.GetAveragePing()
		if ap > worst_ping {
			worst_ping = ap
			worst_conn = v
		}
	}
	if worst_conn != nil {
		if dbg > 0 {
			println("Droping slowest peer", worst_conn.PeerAddr.Ip(), "/", worst_ping, "ms")
		}
		worst_conn.Broken = true
		CountSafe("PeersDropped")
	}
	mutex.Unlock()
}


func (c *oneConnection) TryPing() {
	if c.node.version>60000 && c.PingInProgress == nil && time.Now().After(c.NextPing) {
		/*&&len(c.send.buf)==0 && len(c.GetBlocksInProgress)==0*/
		c.PingInProgress = make([]byte, 8)
		rand.Read(c.PingInProgress[:])
		c.SendRawMsg("ping", c.PingInProgress)
		c.LastPingSent = time.Now()
		//println(c.PeerAddr.Ip(), "ping...")
		return
	}
}
