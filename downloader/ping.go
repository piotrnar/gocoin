package main

import (
//	"fmt"
	"time"
	"sync"
	"bytes"
	"crypto/rand"
//	"encoding/hex"
	"encoding/binary"
	"github.com/piotrnar/gocoin/lib/btc"
)


const (
	PING_FETCH_BLOCKS = 250
	PING_TIMEOUT = 1*time.Second
	PING_SAMPLES = 8
	DROP_SLOW_EVERY = 10*time.Second
)

var (
	RunPings bool
	PingMutex sync.Mutex

	PingInProgress uint32
	PingSequence uint32
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
	c.ping.historyIdx++
	if c.ping.historyIdx >= PING_SAMPLES {
		c.ping.historyIdx = 0
	}
	c.ping.timeSent = time.Now()
	PingMutex.Lock()
	if PingInProgress==c.id {
		PingInProgress = 0
	}
	PingMutex.Unlock()
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


func (c *one_net_conn) block_pong(d []byte) {
	if len(d)>80 {
		c.ping.Lock()
		defer c.ping.Unlock()
		if c.ping.lastBlock!=nil {
			c.ping.bytes += uint(len(d))
			h := btc.NewSha2Hash(d[:80])
			if h.Equal(c.ping.lastBlock) {
				//fmt.Println(c.peerip, "bl_pong", c.ping.seq, c.ping.bytes, time.Now().Sub(c.ping.timeSent))
				c.ping.lastBlock = nil
				c.ping.bytes = 0
				c.store_ping_result()
			}
		}
	}
}


func (c *one_net_conn) ping_idle() {
	c.ping.Lock()
	if c.ping.inProgress {
		if time.Now().After(c.ping.timeSent.Add(PING_TIMEOUT)) {
			c.store_ping_result()
			c.ping.Unlock()
			//fmt.Println(c.peerip, "ping timeout", c.ping.seq)
		} else {
			c.ping.Unlock()
			time.Sleep(time.Millisecond)
		}
	} else if c.ping.now {
		//fmt.Println("ping", c.peerip, c.ping.seq)
		c.ping.inProgress = true
		c.ping.timeSent = time.Now()
		c.ping.now = false
		if false {
			rand.Read(c.ping.pattern[:])
			c.ping.Unlock()
			c.sendmsg("ping", c.ping.pattern[:])
		} else {
			b := new(bytes.Buffer)
			btc.WriteVlen(b, PING_FETCH_BLOCKS)
			BlocksMutex.Lock()
			for i:=uint32(1); ; i++ {
				binary.Write(b, binary.LittleEndian, uint32(2))
				btg := BlocksToGet[i]
				b.Write(btg[:])
				if i==PING_FETCH_BLOCKS {
					c.ping.lastBlock = btc.NewUint256(btg[:])
					break;
				}
			}
			BlocksMutex.Unlock()
			c.ping.bytes = 0
			c.ping.Unlock()
			c.sendmsg("getdata", b.Bytes())
			//fmt.Println("ping sent", c.ping.lastBlock.String())
		}
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
		//fmt.Println(conn2drop.peerip, "- slowest")
		COUNTER("PDRO")
		conn2drop.setbroken(true)
	}
	open_connection_mutex.Unlock()
}


func ping_next_host() {
	PingMutex.Lock()
	defer PingMutex.Unlock()
	if PingInProgress==0 {
		open_connection_mutex.Lock()
		defer open_connection_mutex.Unlock()
		var last_node *one_net_conn
		var minseq = PingSequence+1
		for _, v := range open_connection_list {
			v.ping.Lock()
			if !v.connected_at.IsZero() && v.ping.seq < minseq {
				minseq = v.ping.seq
				last_node = v
			}
			v.ping.Unlock()
		}
		if last_node!=nil {
			PingSequence++
			PingInProgress = last_node.id
			last_node.ping.Lock()
			last_node.ping.seq = PingSequence
			last_node.ping.now = true
			last_node.ping.Unlock()
		}
	}
}

func do_pings() {
	SetRunPings(true)

	next_drop := time.Now().Add(DROP_SLOW_EVERY)

	ping_timeout := time.Now().Add(15*time.Minute)  // auto continue after 15 minutes
	for GetRunPings() {
		if !add_new_connections() {
			time.Sleep(2e8)
		}

		time.Sleep(1e8)
		ping_next_host()

		if time.Now().After(next_drop) {
			drop_longest_ping()
			next_drop = next_drop.Add(DROP_SLOW_EVERY)
		}

		if time.Now().After(ping_timeout) {
			SetRunPings(false)
			break
		}
	}
	PingMutex.Lock()
	PingInProgress = 0
	PingMutex.Unlock()
}
