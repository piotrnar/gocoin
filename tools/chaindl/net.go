package main

import (
	"net"
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
	open_connection []*one_net_conn
	LastBlock struct {
		sync.Mutex
		Node *btc.BlockTreeNode
	}

	PendingHdrs map[[32]byte] bool = make(map[[32]byte] bool)
	PendingHdrsLock sync.Mutex
)


type one_net_cmd struct {
	cmd string
	pl []byte
}


type one_net_conn struct {
	peerip string

	inprogress bool

	// Source of this IP:
	broken bool // flag that the conenction has been broken / shall be disconnected
	closed_s bool
	closed_r bool

	net.Conn
	verackgot bool

	// Messages reception state machine:
	/*recv struct {
		buf []*one_net_cmd
	}*/

	// Message sending state machine:
	send struct {
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

	println("send", cmd, len(sbuf), "...")
	c.send.buf = append(c.send.buf, sbuf...)
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


func (c *one_net_conn) getheaders() {
	var b [4+1+32+32]byte
	binary.LittleEndian.PutUint32(b[0:4], Version)
	b[4] = 1 // one inv
	LastBlock.Mutex.Lock()
	copy(b[5:37], LastBlock.Node.BlockHash.Hash[:])
	LastBlock.Mutex.Unlock()
	c.sendmsg("getheaders", b[:])
}


/*
func (c *one_net_conn) getheaders() {
	b := new(bytes.Buffer)
	PendingHdrsLock.Lock()
	binary.Write(b, binary.LittleEndian, uint32(Version))
	btc.WriteVlen(b, len(len(PendingHdrs)))
	PendingHdrsLock.Unlock()
}
*/


func (c *one_net_conn) idle() {
	if c.verackgot && !c.inprogress {
		c.getheaders()
		c.inprogress = true
	} else {
		time.Sleep(time.Millisecond)
	}
}


func (c *one_net_conn) run_send() {
	c.sendver()
	for !c.broken {
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
		} else {
			c.idle()
		}
	}
	c.closed_s = true
	println(c.peerip, "closing sender")
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


/*
func (c *one_net_conn) gotinv(d []byte) {
	var typ uint32
	var id [32]byte
	b := bytes.NewReader(d)
	cnt, er := btc.ReadVLen(b)
	if er != nil {
		return
	}
	for i := uint64(0); i < cnt; i++ {
		er = binary.Read(b, binary.LittleEndian, &typ)
		if er != nil {
			return
		}
		if _, er = b.Read(id[:]); er != nil {
			return
		}
		if typ==2 {
			PendingHdrsLock.Lock()
			PendingHdrs[id] = true
			PendingHdrsLock.Unlock()
		}
	}
}
*/
func (c *one_net_conn) headers(d []byte) {
	var hdr [81]byte
	b := bytes.NewReader(d)
	cnt, er := btc.ReadVLen(b)
	if er != nil {
		return
	}
	for i := uint64(0); i < cnt; i++ {
		if _, er = b.Read(hdr[:]); er != nil {
			return
		}
		bl, er := btc.NewBlock(hdr[:])
		if er == nil {
			er, dos, later := BlockChain.CheckBlock(bl)
			if er == nil {
				println("got block", bl.Hash.String())
			} else {
				println(er.Error(), dos, later)
			}
		}
	}
}


func (c *one_net_conn) run_recv() {
	for !c.broken {
		msg := c.readmsg()
		if msg==nil {
			println(c.peerip, "- broken when reading")
			c.broken = true
		} else {
			switch msg.cmd {
				case "verack":
					c.verackgot = true

				case "headers":
					if c.inprogress {
						c.headers(msg.pl)
						c.inprogress = false
					}

				default:
					println(c.peerip, "received", msg.cmd, len(msg.pl))
			}
		}
	}
	c.closed_r = true
	println(c.peerip, "closing receiver")
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
	open_connection = append(open_connection, res)
	go res.run_send()
	go res.run_recv()
	return res
}


func get_headers() {
	LastBlock.Node = BlockChain.BlockTreeEnd
	println("get_headers...")
	for {
		time.Sleep(1e9)
	}
}
