package main

import (
	"net"
	"fmt"
	"time"
	"sync"
	"bytes"
	"strings"
//	"encoding/hex"
	"encoding/binary"
	"github.com/piotrnar/gocoin/btc"
)


const (
	UserAgent = "/Satoshi:0.8.5/"
	Version = 70001
	Services = uint64(0x00000001)
)

var (
	open_connection_list map[[4]byte] *one_net_conn = make(map [[4]byte] *one_net_conn)
	open_connection_mutex sync.Mutex
)


type one_net_cmd struct {
	cmd string
	pl []byte
}


type one_net_conn struct {
	peerip string
	ip4 [4]byte

	_hdrsinprogress bool

	// Source of this IP:
	_broken bool // flag that the conenction has been broken / shall be disconnected
	closed_s bool
	closed_r bool

	net.Conn
	verackgot bool

	// Message sending state machine:
	send struct {
		buf []byte
	}

	blockinprogress map[[32]byte] bool
	last_blk_rcvd time.Time
	connected_at time.Time
	bytes_received uint64

	sync.Mutex
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


func (c *one_net_conn) sendmsg(cmd string, pl []byte) (e error) {
	sbuf := make([]byte, 24+len(pl))

	binary.LittleEndian.PutUint32(sbuf[0:4], Version)
	copy(sbuf[0:4], Magic[:])
	copy(sbuf[4:16], cmd)
	binary.LittleEndian.PutUint32(sbuf[16:20], uint32(len(pl)))

	sh := btc.Sha2Sum(pl[:])
	copy(sbuf[20:24], sh[:4])
	copy(sbuf[24:], pl)

	//println("send", cmd, len(sbuf), "...")
	c.Mutex.Lock()
	c.send.buf = append(c.send.buf, sbuf...)
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


func (c *one_net_conn) bps() (res float64) {
	c.Lock()
	res = 1e9 * float64(c.bytes_received) / float64(time.Now().Sub(c.connected_at))
	c.Unlock()
	return
}


func (c *one_net_conn) readmsg() *one_net_cmd {
	var (
		hdr [24]byte
		hdr_len int
		pl_len uint32 // length taken from the message header
		cmd string
		dat []byte
		datlen uint32
	)

	for hdr_len < 24 {
		n, e := c.Read(hdr[hdr_len:])
		if e != nil {
			return nil
		}
		c.Lock()
		c.bytes_received += uint64(n)
		c.Unlock()
		hdr_len += n
		if hdr_len>=4 && !bytes.Equal(hdr[:4], Magic[:]) {
			println(c.peerip, "NetBadMagic")
			return nil
		}
	}
	pl_len = binary.LittleEndian.Uint32(hdr[16:20])
	cmd = strings.TrimRight(string(hdr[4:16]), "\000")

	if pl_len > 0 {
		dat = make([]byte, pl_len)
		for datlen < pl_len {
			n, e := c.Read(dat[datlen:])
			if e != nil {
				return nil
			}
			if n > 0 {
				datlen += uint32(n)
				c.Lock()
				c.bytes_received += uint64(n)
				c.Unlock()
			}
		}
	}

	sh := btc.Sha2Sum(dat)
	if !bytes.Equal(hdr[20:24], sh[:4]) {
		println(c.peerip, "Msg checksum error")
		return nil
	}

	res := new(one_net_cmd)
	res.cmd = cmd
	res.pl = dat
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
		//println("-", c.peerip)
		open_connection_mutex.Lock()
		delete(open_connection_list, c.ip4)
		COUNTER("DROP_PEER")
		open_connection_mutex.Unlock()
		for k, _ := range c.blockinprogress {
			BlocksMutex.Lock()
			bip := BlocksInProgress[k]
			if bip!=nil {
				//println(" block", bip.Height, btc.NewUint256(k[:]).String(), "no more. ", bip.Count, "left")
				bip.Count--
				if bip.Count==0 && bip.Height-1<BlocksIndex {
					//println("BlocksIndex:", BlocksIndex, "->", BlocksIndex-1)
					BlocksIndex = BlocksIndex-1
				}
			}
			BlocksMutex.Unlock()
		}
	}
}


func (c *one_net_conn) run_recv() {
	for !c.isbroken() {
		msg := c.readmsg()
		if msg==nil {
			//println(c.peerip, "- broken when reading")
			c.setbroken(true)
		} else {
			switch msg.cmd {
				case "verack":
					c.Mutex.Lock()
					c.verackgot = true
					c.Mutex.Unlock()
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
					c.block(msg.pl)

				case "version":

				case "addr":
					parse_addr(msg.pl)

				case "inv":

				default:
					println(c.peerip, "received", msg.cmd, len(msg.pl))
			}
		}
	}
	//println(c.peerip, "closing receiver")
	c.Mutex.Lock()
	c.closed_r = true
	c.cleanup()
	c.Mutex.Unlock()
}


func (c *one_net_conn) idle() {
	c.Mutex.Lock()
	if c.verackgot {
		c.Mutex.Unlock()
		if !c.hdr_idle() && GetDoBlocks() {
			c.blk_idle()
		}
	} else {
		c.Mutex.Unlock()
		time.Sleep(time.Millisecond)
	}
}


func (c *one_net_conn) run_send() {
	c.sendver()
	for !c.isbroken() {
		c.Mutex.Lock()
		if len(c.send.buf) > 0 {
			c.Mutex.Unlock()
			c.SetWriteDeadline(time.Now().Add(time.Millisecond))
			n, e := c.Write(c.send.buf)
			if e != nil {
				if nerr, ok := e.(net.Error); ok && nerr.Timeout() {
					e = nil
				} else {
					c.setbroken(true)
				}
			} else {
				c.send.buf = c.send.buf[n:]
			}
		} else {
			c.Mutex.Unlock()
			c.idle()
		}
	}
	//println(c.peerip, "closing sender")
	c.Mutex.Lock()
	c.closed_s = true
	c.cleanup()
	c.Mutex.Unlock()
}



func (res *one_net_conn) connect() {
	con, er := net.Dial("tcp4", res.peerip+":8333")
	if er != nil {
		res.setbroken(true)
		res.closed_r = true
		res.closed_s = true
		res.cleanup()
		//println(er.Error())
		return
	}
	res.Mutex.Lock()
	res.Conn = con
	//println(res.peerip, "connected")
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
	res.blockinprogress = make(map[[32]byte] bool)
	open_connection_mutex.Lock()
	AddrDatbase[ip4] = true
	open_connection_list[ip4] = res
	open_connection_mutex.Unlock()
	go res.connect()
	return res
}


func add_new_connections() {
	if open_connection_count() < MAX_CONNECTIONS {
		AddrMutex.Lock()
		defer AddrMutex.Unlock()
		for k, v := range AddrDatbase {
			if !v {
				new_connection(k)
				COUNTER("CONN_PEER")
				if open_connection_count() >= MAX_CONNECTIONS {
					return
				}
			}
		}
	}
}
