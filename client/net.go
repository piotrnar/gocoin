package main

import (
	"bytes"
	"errors"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"net"
	"os"
	"strings"
	"time"
    "github.com/piotrnar/gocoin/btc"
)


const (
	MaxCons = 8
)


var (
	openCons map[uint64]*oneConnection = make(map[uint64]*oneConnection, MaxCons)
)

type oneConnection struct {
	addr *onePeer
	
	broken bool // maker that the conenction has been broken

	listen bool
	*net.TCPConn
	
	hdr [24]byte
	hdr_len int

	dat []byte
	datlen uint32

	// Data from the version message
	node struct {
		version uint32
		services uint64
		timestamp uint64
		height uint32
		agent string
	}
}


type BCmsg struct {
	cmd string
	pl  []byte
}


func (c *oneConnection) SendRawMsg(cmd string, pl []byte) (e error) {
	var b [20]byte

	binary.LittleEndian.PutUint32(b[0:4], Version)
	copy(b[0:4], Magic[:])
	copy(b[4:16], cmd)
	binary.LittleEndian.PutUint32(b[16:20], uint32(len(pl)))
	_, e = c.TCPConn.Write(b[:20])
	if e != nil {
		println("SendRawMsg 1", e.Error())
		c.broken = true
		return
	}

	sh := btc.Sha2Sum(pl[:])
	_, e = c.TCPConn.Write(sh[:4])
	if e != nil {
		println("SendRawMsg 2", e.Error())
		c.broken = true
		return
	}

	_, e = c.TCPConn.Write(pl[:])
	if e != nil {
		println("SendRawMsg 3", e.Error())
		c.broken = true
	}

	return
}


func putaddr(b *bytes.Buffer, a string) {
	var i1, i2, i3, i4, p int

	n, e := fmt.Sscanf(a, "%d.%d.%d.%d:%d", &i1, &i2, &i3, &i4, &p)
	if e != nil || n != 5 {
		println("Incorrect address:", a)
		os.Exit(1)
	}

	b.Write(Services)
	b.Write([]byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0xFF, 0xFF})

	b.WriteByte(byte(i1))
	b.WriteByte(byte(i2))
	b.WriteByte(byte(i3))
	b.WriteByte(byte(i4))
	b.WriteByte(byte(p >> 8))
	b.WriteByte(byte(p & 0xff))
}


func (c *oneConnection) SendVersion() {
	b := bytes.NewBuffer([]byte{})

	WriteLSB(b, Version, 4)
	b.Write(Services)
	WriteLSB(b, uint64(time.Now().Unix()), 8)

	putaddr(b, c.TCPConn.RemoteAddr().String())
	putaddr(b, c.TCPConn.LocalAddr().String())

	var r [8]byte
	rand.Read(r[:])
	b.Write(r[:])

	b.WriteByte(0)
	WriteLSB(b, 0, 4) // last block

	c.SendRawMsg("version", b.Bytes())
}


func (c *oneConnection) HandleError(e error) (error) {
	if nerr, ok := e.(net.Error); ok && nerr.Timeout() {
		//fmt.Println("Just a timeout - ignore")
		return nil
	}
	println("HandleError:", e.Error())
	c.hdr_len = 0
	c.dat = nil
	c.broken = true
	return e
}


func (c *oneConnection) FetchMessage() (*BCmsg) {
	var e error
	var n int

	// Try for 1 millisecond and timeout if full msg not received
	c.TCPConn.SetReadDeadline(time.Now().Add(time.Millisecond))

	for c.hdr_len < 24 {
		n, e = c.TCPConn.Read(c.hdr[c.hdr_len:24])
		c.hdr_len += n
		if e != nil {
			c.HandleError(e)
			return nil
		}
		if c.hdr_len>=4 && !bytes.Equal(c.hdr[:4], Magic[:]) {
			println("FetchMessage: Proto out of sync")
			c.broken = true
			return nil
		}
	}

	dlen :=  binary.LittleEndian.Uint32(c.hdr[16:20])
	if dlen > 0 {
		if c.dat == nil {
			c.dat = make([]byte, dlen)
			c.datlen = 0
		}
		for c.datlen < dlen {
			n, e = c.TCPConn.Read(c.dat[c.datlen:])
			c.datlen += uint32(n)
			if e != nil {
				c.HandleError(e)
				return nil
			}
		}
	}

	sh := btc.Sha2Sum(c.dat)
	if !bytes.Equal(c.hdr[20:24], sh[:4]) {
		println("Msg checksum error")
		c.hdr_len = 0
		c.dat = nil
		c.broken = true
		return nil
	}

	ret := new(BCmsg)
	ret.cmd = strings.TrimRight(string(c.hdr[4:16]), "\000")
	ret.pl = c.dat
	c.dat = nil
	c.hdr_len = 0

	return ret
}


func (c *oneConnection) VerMsg(pl []byte) error {
	if len(pl) >= 46 {
		c.node.version = binary.LittleEndian.Uint32(pl[0:4])
		c.node.services = binary.LittleEndian.Uint64(pl[4:12])
		c.node.timestamp = binary.LittleEndian.Uint64(pl[12:20])
		if len(pl) >= 86 {
			//fmt.Println("From:", btc.NewNetAddr(pl[46:72]).String())
			//fmt.Println("Nonce:", hex.EncodeToString(pl[72:80]))
			le, of := btc.VLen(pl[80:])
			of += 80
			c.node.agent = string(pl[of:of+le])
			of += le
			if len(pl) >= of+4 {
				c.node.height = binary.LittleEndian.Uint32(pl[of:of+4])
				/*of += 4
				if len(pl) >= of+1 {
					fmt.Println("Relay:", pl[of])
				}*/
			}
		}
	} else {
		return errors.New("Version message too short")
	}
	c.SendRawMsg("verack", []byte{})
	if c.listen {
		c.SendVersion()
	}
	return nil
}


func (c *oneConnection) GetBlocks(lastbl []byte) {
	//println("GetBlocks since", btc.NewUint256(lastbl).String())
	var b [4+1+32+32]byte
	binary.LittleEndian.PutUint32(b[0:4], Version)
	b[4] = 1 // only one locator
	copy(b[5:37], lastbl)
	// the remaining bytes should be filled with zero
	c.SendRawMsg("getblocks", b[:])
}


func (c *oneConnection) ProcessInv(pl []byte) {
	if dbg>0 {
		println("ProcessInv", len(pl))
	}
	
	if len(pl) < 37 {
		println("inv payload too short")
		return
	}
	
	cnt, of := btc.VLen(pl)
	if len(pl) != of + 36*cnt {
		println("inv payload length mismatch", len(pl), of, cnt)
	}

	var blocks2get [][32]byte
	for cnt>0 {
		typ := binary.LittleEndian.Uint32(pl[of:of+4])
		if typ==2 && InvsNotify(pl[of+4:of+36]) {
			var inv [32]byte
			copy(inv[:], pl[of+4:of+36])
			blocks2get = append(blocks2get, inv)
		}
		of+= 36
		cnt--
	}
	if len(blocks2get) > 0 {
		msg := make([]byte, 9/*maxvlen*/+len(blocks2get)*36)
		le := btc.PutVlen(msg, len(blocks2get))
		for i := range blocks2get {
			binary.LittleEndian.PutUint32(msg[le:le+4], 2)
			copy(msg[le+4:le+36], blocks2get[i][:])
			le += 36
		}
		if dbg>0 {
			println("getdata for", len(blocks2get), le, "blocks")
		}
		c.SendRawMsg("getdata", msg[:le])
	}
	return
}


func blockReceived(b []byte) {
	msg := new(command)
	msg.src = "net"
	msg.str = "bl"
	msg.dat = b
	cmdChannel <- msg
}


func (c *oneConnection) GetBlockData(h []byte) {
	var b [1+4+32]byte
	b[0] = 1 // One inv
	b[1] = 2 // Block
	copy(b[5:37], h[:32])
	c.SendRawMsg("getdata", b[:])
}


func (c *oneConnection) Tick() {
	if tmp := blocksNeeded(); tmp != nil {
		c.GetBlocks(tmp)
		return
	}

	if tmp := blockDataNeeded(); tmp != nil {
		c.GetBlockData(tmp)
	}
}


func do_one_connection(c *oneConnection) {
	c.SendVersion()

	ver_ack_received := false

	for {
		cmd := c.FetchMessage()
		if c.broken {
			c.addr.Failed()
			break
		}
		
		if cmd==nil {
			if ver_ack_received {
				c.Tick()
			}
			continue
		}

		c.addr.GotData(24+len(cmd.pl))

		switch cmd.cmd {
			case "version":
				er := c.VerMsg(cmd.pl)
				if er != nil {
					println("version:", er.Error())
					c.addr.Failed()
					c.broken = true
				}

			case "verack":
				//fmt.Println("Received Ver ACK")
				ver_ack_received = true

			case "inv":
				c.ProcessInv(cmd.pl)
			
			case "tx": //ParseTx(cmd.pl)
				println("tx unexpected here (now)")
				c.broken = true
			
			case "addr": ParseAddr(cmd.pl)
			
			case "block": //ParseBlock(cmd.pl)
				blockReceived(cmd.pl)

			case "alert": // do nothing

			default:
				println(cmd.cmd)
		}
	}
	if dbg>0 {
		println("Disconnected from", c.addr.Ip())
	}
	c.TCPConn.Close()
}


func connectionActive(ad *onePeer) (yes bool) {
	mutex.Lock()
	_, yes = openCons[ad.UniqID()]
	mutex.Unlock()
	return
}


func do_network(ad *onePeer) {
	var e error
	conn := new(oneConnection)
	conn.addr = ad
	mutex.Lock()
	openCons[ad.UniqID()] = conn
	mutex.Unlock()
	go func() {
		conn.TCPConn, e = net.DialTCP("tcp4", nil, &net.TCPAddr{
			IP: net.IPv4(ad.Ip4[0], ad.Ip4[1], ad.Ip4[2], ad.Ip4[3]),
			Port: int(ad.Port)})
		if e == nil {
			ad.Connected()
			if dbg>0 {
				println("Connected to", ad.Ip())
			}
			do_one_connection(conn)
		} else {
			if dbg>0 {
				println("Could not connect to", ad.Ip())
			}
			//println(e.Error())
			ad.Failed()
		}
		mutex.Lock()
		delete(openCons, ad.UniqID())
		mutex.Unlock()
	}()
}


func network_process() {
	for {
		mutex.Lock()
		conn_cnt := len(openCons)
		mutex.Unlock()
		if conn_cnt < MaxCons {
			ad := getBestPeer()
			if ad != nil {
				do_network(ad)
			} else if *proxy=="" {
				println("no new peers", len(openCons), MaxCons)
			}
		}
		time.Sleep(250e6)
	}
}

func net_stats() {
	mutex.Lock()
	println(len(openCons), "active net connections:")
	for _, v := range openCons {
		println(" ", v.addr.Ip(), "\t", v.addr.BytesReceived, "bts  \tver:",
			v.node.version, v.node.agent, "\t", v.node.height)
	}
	mutex.Unlock()
}
