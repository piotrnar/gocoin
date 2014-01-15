package main

import (
	"time"
	"sync"
	"bytes"
	"crypto/rand"
)


const (
	PING_PERIOD  = 5*time.Second
	PING_TIMEOUT = 5*time.Second
	PING_SAMPLES = 8
	DROP_SLOW_EVERY = 9*time.Second
)

var (
	RunPings bool
	PingMutex sync.Mutex
)


func GetRunPings() (res bool) {
	PingMutex.Lock()
	res = RunPings
	PingMutex.Unlock()
	return
}


func SetRunPings(res bool) {
	PingMutex.Lock()
	RunPings = res
	PingMutex.Unlock()
}

// Call it with the mutex locked
func (c *one_net_conn) store_ping_result() {
	c.ping.historyMs[c.ping.historyIdx] = uint(time.Now().Sub(c.ping.timeSent) / time.Millisecond)
	c.ping.inProgress = false
	c.ping.cnt++
	c.ping.historyIdx++
	if c.ping.historyIdx >= PING_SAMPLES {
		c.ping.historyIdx = 0
	}
	c.ping.timeSent = time.Now()
}


func (c *one_net_conn) pong(d []byte) {
	c.ping.Lock()
	if c.ping.inProgress && len(d)==8 && bytes.Equal(d, c.ping.pattern[:]) {
		c.store_ping_result()
	}
	c.ping.Unlock()
}


func (c *one_net_conn) avg_ping() (sum uint) {
	var cnt uint
	c.ping.Lock()
	for i := range c.ping.historyMs {
		if c.ping.historyMs[i]>0 {
			sum += c.ping.historyMs[i]
			cnt++
		}
	}
	c.ping.Unlock()
	if cnt>0 {
		sum /= cnt
	}
	return
}


func (c *one_net_conn) ping_idle() {
	c.ping.Lock()
	if c.ping.inProgress {
		if time.Now().After(c.ping.timeSent.Add(PING_TIMEOUT)) {
			c.store_ping_result()
			c.ping.Unlock()
		} else {
			c.ping.Unlock()
			time.Sleep(time.Millisecond)
		}
	} else if c.ping.timeSent.IsZero() || c.ping.timeSent.Add(PING_PERIOD).Before(time.Now()) {
		//println("ping", c.peerip)
		c.ping.inProgress = true
		c.ping.timeSent = time.Now()
		rand.Read(c.ping.pattern[:])
		c.ping.Unlock()
		c.sendmsg("ping", c.ping.pattern[:])
	} else {
		c.ping.Unlock()
		time.Sleep(10*time.Millisecond)
	}
}


func drop_longest_ping() {
	var conn2drop *one_net_conn
	var maxping uint
	open_connection_mutex.Lock()
	for _, v := range open_connection_list {
		cp := v.avg_ping()
		if cp > maxping {
			maxping = cp
			conn2drop = v
		}
	}
	if conn2drop!=nil {
		//println(conn2drop.peerip, "- slowest")
		COUNTER("PDRO")
		conn2drop.setbroken(true)
	}
	open_connection_mutex.Unlock()
}


func do_pings() {
	SetRunPings(true)

	next_drop := time.Now().Add(DROP_SLOW_EVERY)

	for GetRunPings() {
		if !add_new_connections() {
			time.Sleep(2e8)
		}
		if time.Now().After(next_drop) {
			//println("drop...")
			drop_longest_ping()
			next_drop = next_drop.Add(DROP_SLOW_EVERY)
		}
	}
}
