package main

import (
	"net"
	"time"
	"sync"
	"bytes"
	"strings"
	"encoding/hex"
	"encoding/binary"
	"github.com/piotrnar/gocoin/btc"
)


const (
	UserAgent = "/Satoshi:0.8.5/"
	MaxSendBufferSize = 16*1024*1024 // If you have more than this in the send buffer, disconnect
	Version = 70001
	Services = uint64(0x00000001)
)

var (
	open_connection []*one_net_conn
)


type one_net_cmd struct {
	cmd string
	pl []byte
}


type one_net_conn struct {
	peerip string

	// Source of this IP:
	broken bool // flag that the conenction has been broken / shall be disconnected
	closed_s bool // flag that the conenction has been broken / shall be disconnected
	closed_r bool // flag that the conenction has been broken / shall be disconnected

	net.Conn
	VerackReceived bool

	// Messages reception state machine:
	recv struct {
		sync.Mutex
		buf []*one_net_cmd
	}

	// Message sending state machine:
	send struct {
		sync.Mutex
		buf []byte
	}
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

	c.send.Mutex.Lock()
	println("send", cmd, hex.EncodeToString(sbuf), "...")
	c.send.buf = append(c.send.buf, sbuf...)
	c.send.Mutex.Unlock()
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


func (c *one_net_conn) run_send() {
	for !c.broken {
		c.send.Mutex.Lock()
		if len(c.send.buf) > 0 {
			c.SetWriteDeadline(time.Now().Add(time.Millisecond))
			n, e := c.Write(c.send.buf)
			if e != nil {
				if nerr, ok := e.(net.Error); ok && nerr.Timeout() {
					e = nil
				} else {
					c.broken = true
				}
			} else {
				c.send.buf = c.send.buf[n:]
				println(c.peerip, n, "sent")
			}
		}
		c.send.Mutex.Unlock()
	}
	c.closed_s = true
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
		hdr_len += n
		if hdr_len>=4 && !bytes.Equal(hdr[:4], Magic[:]) {
			println("NetBadMagic")
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


func (c *one_net_conn) run_recv() {
	for !c.broken {
		msg := c.readmsg()
		if msg==nil {
			println(c.peerip, "- broken when reading")
			c.broken = true
		} else {
			c.recv.Mutex.Lock()
			c.recv.buf = append(c.recv.buf, msg)
			c.recv.Mutex.Unlock()
			println(c.peerip, "received", msg.cmd, len(msg.pl))
		}
	}
	c.closed_r = true
}


func new_connection(ip string) *one_net_conn {
	var er error
	res := new(one_net_conn)
	res.peerip = ip
	res.Conn, er = net.Dial("tcp4", ip+":8333")
	if er != nil {
		println(er.Error())
		return nil
	}
	println("connected to", res.peerip)
	res.sendver()
	open_connection = append(open_connection, res)
	go res.run_send()
	go res.run_recv()
	return res
}


func get_headers() {
	println("get_headers...")
	for {
		time.Sleep(1e9)
	}
}
