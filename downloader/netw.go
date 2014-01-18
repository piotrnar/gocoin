package main

import (
	"net"
	"fmt"
	"time"
	"sync"
	"bytes"
	"strings"
	"sync/atomic"
	"encoding/binary"
	"github.com/piotrnar/gocoin/btc"
)


const (
	UserAgent = "/Satoshi:0.8.5/"
	Version = 70001
	Services = uint64(0x00000001)

	DIAL_TIMEOUT = 3*time.Second
)

var (
	open_connection_list map[[4]byte] *one_net_conn = make(map [[4]byte] *one_net_conn)
	open_connection_mutex sync.Mutex
	curid uint32
)


type one_net_cmd struct {
	cmd string
	pl []byte
}


type one_net_conn struct {
	id uint32

	peerip string
	ip4 [4]byte

	_hdrsinprogress bool

	// Source of this IP:
	_broken bool // flag that the conenction has been broken / shall be disconnected
	closed_s bool
	closed_r bool

	net.Conn

	// Message receiving state machine:
	recv struct {
		hdr [24]byte
		hdr_len int
		pl_len uint32 // length taken from the message header
		cmd string
		dat []byte
		datlen uint32
	}


	// Message sending state machine:
	send struct {
		buf []byte
	}

	inprogress uint32

	last_blk_rcvd time.Time
	connected_at time.Time
	bytes_received uint64

	sync.Mutex

	ping struct {
		sync.Mutex

		now bool
		seq uint32

		inProgress bool
		timeSent time.Time
		pattern [8]byte
		lastBlock *btc.Uint256
		bytes uint

		historyMs [PING_SAMPLES]uint
		historyIdx int
	}
}


func (c *one_net_conn) isconnected() (res bool) {
	c.Lock()
	res = !c._broken && !c.connected_at.IsZero()
	c.Unlock()
	return
}


func (c *one_net_conn) isbroken() (res bool) {
	c.Lock()
	res = c._broken
	c.Unlock()
	return
}


func (c *one_net_conn) setbroken(res bool) {
	c.Lock()
	c._broken = res
	c.Unlock()
}


func (c *one_net_conn) sendbuflen() (sbl int) {
	c.Lock()
	sbl = len(c.send.buf)
	c.Unlock()
	return
}


func (c *one_net_conn) sendmsg(cmd string, pl []byte) (e error) {
	sbuf := make([]byte, 24+len(pl))

	binary.LittleEndian.PutUint32(sbuf[0:4], Version)
	copy(sbuf[0:4], Magic[:])
	copy(sbuf[4:16], cmd)
	binary.LittleEndian.PutUint32(sbuf[16:20], uint32(len(pl)))

	sh := btc.Sha2Sum(pl[:])
	copy(sbuf[20:24], sh[:4])
	copy(sbuf[24:], pl)

	c.Mutex.Lock()
	c.send.buf = append(c.send.buf, sbuf...)
	//fmt.Println("...", len(c.send.buf))
	c.Mutex.Unlock()
	return
}


func (c *one_net_conn) sendver() {
	b := bytes.NewBuffer([]byte{})
	binary.Write(b, binary.LittleEndian, uint32(Version))
	binary.Write(b, binary.LittleEndian, uint64(Services))
	binary.Write(b, binary.LittleEndian, uint64(time.Now().Unix()))

	// Remote Addr
	binary.Write(b, binary.LittleEndian, Services)
	b.Write(bytes.Repeat([]byte{0}, 12)) // ip6
	b.Write(bytes.Repeat([]byte{0}, 4)) // ip4
	binary.Write(b, binary.LittleEndian, uint16(8333)) // port

	b.Write(bytes.Repeat([]byte{0}, 26)) // Local Addr
	b.Write(bytes.Repeat([]byte{0}, 8)) // nonce
	b.WriteByte(byte(len(UserAgent)))
	b.Write([]byte(UserAgent))
	binary.Write(b, binary.LittleEndian, uint32(0))  // Last Block Height
	b.WriteByte(0)  // don't notify me about txs
	c.sendmsg("version", b.Bytes())
}

// Lock the mutex before calling it
func (c *one_net_conn) bps() (res float64) {
	res = 1e9 * float64(c.bytes_received) / float64(time.Now().Sub(c.connected_at))
	return
}


func (c *one_net_conn) readmsg() *one_net_cmd {
	c.SetReadDeadline(time.Now().Add(10*time.Millisecond))
	if c.recv.hdr_len<24 {
		for {
			n, e := c.Read(c.recv.hdr[c.recv.hdr_len:])
			if e != nil {
				if nerr, ok := e.(net.Error); ok && nerr.Timeout() {
					//COUNTER("HDRT")
				} else {
					c.setbroken(true)
				}
				return nil
			}
			c.Lock()
			c.bytes_received += uint64(n)
			c.Unlock()
			c.recv.hdr_len += n
			if c.recv.hdr_len>=4 {
				if !bytes.Equal(c.recv.hdr[:4], Magic[:]) {
					fmt.Println(c.peerip, "NetBadMagic")
					c.setbroken(true)
					return nil
				}
				if c.recv.hdr_len==24 {
					c.recv.cmd = strings.TrimRight(string(c.recv.hdr[4:16]), "\000")
					c.recv.pl_len = binary.LittleEndian.Uint32(c.recv.hdr[16:20])
					c.recv.datlen = 0
					if c.recv.pl_len > 0 {
						c.recv.dat = make([]byte, c.recv.pl_len)
					}
					break
				}
			}
		}
	}

	for c.recv.datlen < c.recv.pl_len {
		n, e := c.Read(c.recv.dat[c.recv.datlen:])
		if e != nil {
			if nerr, ok := e.(net.Error); ok && nerr.Timeout() {
				//COUNTER("HDRT")
			} else {
				c.setbroken(true)
			}
			return nil
		}
		if n > 0 {
			c.recv.datlen += uint32(n)
			c.Lock()
			c.bytes_received += uint64(n)
			c.Unlock()
		}
	}

	sh := btc.Sha2Sum(c.recv.dat)
	if !bytes.Equal(c.recv.hdr[20:24], sh[:4]) {
		fmt.Println(c.peerip, "Msg checksum error")
		c.setbroken(true)
		return nil
	}

	res := new(one_net_cmd)
	res.cmd = c.recv.cmd
	res.pl = c.recv.dat

	c.recv.hdr_len = 0
	c.recv.dat = nil

	return res
}


func (c *one_net_conn) gethdrsinprogress() (res bool) {
	c.Lock()
	res = c._hdrsinprogress
	c.Unlock()
	return
}


func (c *one_net_conn) sethdrsinprogress(res bool) {
	c.Lock()
	c._hdrsinprogress = res
	c.Unlock()
}


func (c *one_net_conn) cleanup() {
	if c.closed_r && c.closed_s {
		COUNTER("DROP")

		// Cleanup pending ping
		PingMutex.Lock()
		if c.id==PingInProgress {
			fmt.Println(c.peerip, "abort ping")
			PingInProgress = 0
		}
		PingMutex.Unlock()

		// Remove from open connections
		open_connection_mutex.Lock()
		delete(open_connection_list, c.ip4)
		open_connection_mutex.Unlock()

		// Remove peers db
		AddrMutex.Lock()
		delete(AddrDatbase, c.ip4)
		AddrMutex.Unlock()

		// Remove from pending blocks
		BlocksMutex_Lock()
		bi := BlocksIndex
		for k, v := range BlocksInProgress {
			if v.Conns[c.id] {
				delete(v.Conns, c.id)
				if v.Count==1 {
					delete(BlocksInProgress, k)
					if v.Height-1<bi {
						bi = bi-1
					}
				} else {
					v.Count--
				}
			}
		}
		if BlocksIndex!=bi {
			BlocksIndex = bi
			COUNTER("REWI")
		}
		BlocksMutex_Unlock()
	}
}


func (c *one_net_conn) run_recv() {
	var verackgot bool
	for !c.isbroken() {
		if verackgot {
			if !c.hdr_idle() {
				if GetRunPings() {
					c.ping_idle()
				}
				if GetDoBlocks() {
					c.blk_idle()
				}
			}
		}

		msg := c.readmsg()
		if msg==nil {
			//time.Sleep(5*time.Millisecond)
			continue
		}

		switch msg.cmd {
			case "verack":
				verackgot = true
				AddrMutex.Lock()
				if len(AddrDatbase)<2000 {
					c.sendmsg("getaddr", nil)
				}
				AddrMutex.Unlock()

			case "headers":
				if c.gethdrsinprogress() {
					c.sethdrsinprogress(false)
					c.headers(msg.pl)
				}

			case "block":
				if GetRunPings() {
					c.block_pong(msg.pl)
				} else {
					c.block(msg.pl)
				}

			case "version":

			case "addr":
				parse_addr(msg.pl)

			case "inv":

			case "pong":
				if GetRunPings() {
					c.pong(msg.pl)
				}

			default:
				fmt.Println(c.peerip, "received", msg.cmd, len(msg.pl))
		}
	}
	//fmt.Println(c.peerip, "closing receiver")
	c.Mutex.Lock()
	c.closed_r = true
	c.cleanup()
	c.Mutex.Unlock()
}


func (c *one_net_conn) run_send() {
	c.sendver()
	for !c.isbroken() {
		if c.sendbuflen() > 0 {
			c.SetWriteDeadline(time.Now().Add(10*time.Millisecond))
			n, e := c.Write(c.send.buf)
			if e != nil {
				if nerr, ok := e.(net.Error); ok && nerr.Timeout() {
					e = nil
				} else {
					c.setbroken(true)
				}
			} else {
				c.Mutex.Lock()
				c.send.buf = c.send.buf[n:]
				c.Mutex.Unlock()
			}
		} else {
			time.Sleep(10*time.Millisecond)
		}
	}
	//fmt.Println(c.peerip, "closing sender")
	c.Mutex.Lock()
	c.closed_s = true
	c.cleanup()
	c.Mutex.Unlock()
}



func (res *one_net_conn) connect() {
	con, er := net.DialTimeout("tcp4", res.peerip+":8333", DIAL_TIMEOUT)
	if er != nil {
		COUNTER("CERR")
		res.setbroken(true)
		res.closed_r = true
		res.closed_s = true
		res.cleanup()
		//fmt.Println(er.Error())
		return
	}
	res.Mutex.Lock()
	res.Conn = con
	//fmt.Println(res.peerip, "connected")
	go res.run_send()
	go res.run_recv()
	res.connected_at = time.Now()
	res.Mutex.Unlock()
}


// make sure to call it within AddrMutex
func new_connection(ip4 [4]byte) *one_net_conn {
	res := new(one_net_conn)
	res.peerip = fmt.Sprintf("%d.%d.%d.%d", ip4[0], ip4[1], ip4[2], ip4[3])
	res.ip4 = ip4
	res.id = atomic.AddUint32(&curid, 1)
	open_connection_mutex.Lock()
	AddrDatbase[ip4] = true
	open_connection_list[ip4] = res
	open_connection_mutex.Unlock()
	go res.connect()
	return res
}


func add_new_connections() bool {
	if open_connection_count() < MAX_CONNECTIONS {
		AddrMutex.Lock()
		defer AddrMutex.Unlock()
		for k, v := range AddrDatbase {
			if !v {
				new_connection(k)
				COUNTER("CONN")
				if open_connection_count() >= MAX_CONNECTIONS {
					return true
				}
			}
		}
		return true
	}
	return false
}

func close_all_connections() {
	open_connection_mutex.Lock()
	for _, v := range open_connection_list {
		v.setbroken(true)
	}
	open_connection_mutex.Unlock()
	for open_connection_count()>0 {
		time.Sleep(1e8)
	}
}
