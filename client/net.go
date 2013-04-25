package main

import (
	"bytes"
	"errors"
	"sync"
	"crypto/rand"
	"encoding/hex"
	"encoding/binary"
	"fmt"
	"net"
	"os"
	"strings"
	"time"
    "github.com/piotrnar/gocoin/btc"
)


var (
	mutex sync.Mutex
	askForBlocks []byte
	askForData []byte
)

type oneConnection struct {
	listen bool
	*net.TCPConn
	
	hdr [24]byte
	hdr_len int

	dat []byte
	datlen uint32
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
		return
	}

	sh := btc.Sha2Sum(pl[:])
	_, e = c.TCPConn.Write(sh[:4])
	if e != nil {
		return
	}

	_, e = c.TCPConn.Write(pl[:])
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
	c.hdr_len = 0
	c.dat = nil
	return e
}


func (c *oneConnection) FetchMessage() (*BCmsg, error) {
	var e error
	var n int

	c.TCPConn.SetReadDeadline(time.Now().Add(time.Second))
	//println("reading response")

	for c.hdr_len < 24 {
		n, e = c.TCPConn.Read(c.hdr[c.hdr_len:24])
		c.hdr_len = n
		if e != nil {
			return nil, c.HandleError(e)
		}
		if c.hdr_len>=4 && !bytes.Equal(c.hdr[:4], Magic[:]) {
			println("Proto sync...")
			copy(c.hdr[0:c.hdr_len-1], c.hdr[1:c.hdr_len])
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
				return nil, c.HandleError(e)
			}
		}
	}

	sh := btc.Sha2Sum(c.dat)
	if !bytes.Equal(c.hdr[20:24], sh[:4]) {
		c.hdr_len = 0
		c.dat = nil
		return nil, errors.New("Msg checksum error")
	}

	ret := new(BCmsg)
	ret.cmd = strings.TrimRight(string(c.hdr[4:16]), "\000")
	ret.pl = c.dat
	c.dat = nil
	c.hdr_len = 0

	return ret, nil
}


func (c *oneConnection) VerMsg(pl []byte) {
	if len(pl) >= 46 {
		fmt.Println()
		fmt.Printf("Version %d, serv=0x%x, time:%s\n",
			binary.LittleEndian.Uint32(pl[0:4]),
			binary.LittleEndian.Uint64(pl[4:12]),
			time.Unix(int64(binary.LittleEndian.Uint64(pl[12:20])), 0).Format("2006-01-02 15:04:05"),
			)
		fmt.Println("Recv:", btc.NewNetAddr(pl[20:46]).String())
		if len(pl) >= 86 {
			fmt.Println("From:", btc.NewNetAddr(pl[46:72]).String())
			fmt.Println("Nonce:", hex.EncodeToString(pl[72:80]))
			le, of := btc.VLen(pl[80:])
			of += 80
			fmt.Println("Agent:", string(pl[of:of+le]))
			of += le
			if len(pl) >= of+4 {
				fmt.Println("Height:", binary.LittleEndian.Uint32(pl[of:of+4]))
				of += 4
				if len(pl) >= of+1 {
					fmt.Println("Relay:", pl[of])
				}
			}
		}
	} else {
		println("Corrupt version message", hex.EncodeToString(pl[:]))
	}
	c.SendRawMsg("verack", []byte{})
	if c.listen {
		c.SendVersion()
	}
}

func (c *oneConnection) GetBlockData(h []byte) {
	var b [1+4+32]byte
	b[0] = 1 // One inv
	b[1] = 2 // Block
	copy(b[5:37], h[:32])
	c.SendRawMsg("getdata", b[:])
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


func ProcessInv(pl []byte) {
	if len(pl) < 37 {
		println("inv payload too short")
		return
	}
	
	cnt, of := btc.VLen(pl)
	if len(pl) != of + 36*cnt {
		println("inv payload length mismatch", len(pl), of, cnt)
	}

	for cnt>0 {
		typ := binary.LittleEndian.Uint32(pl[of:of+4])
		if typ==2 {
			msg := new(command)
			msg.src = "net"
			msg.str = "invbl"
			msg.dat = pl[of+4:of+36]
			cmdChannel <- msg
		}
		of+= 36
		cnt--
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


func (c *oneConnection) Tick() {
	mutex.Lock()
	if askForBlocks != nil {
		tmp := askForBlocks
		askForBlocks = nil
		mutex.Unlock()
		c.GetBlocks(tmp)
		return
	}
	if askForData != nil {
		tmp := askForData
		askForData = nil
		mutex.Unlock()
		c.GetBlockData(tmp)
		return
	}
	mutex.Unlock()
}


func do_one_connection(c oneConnection) {
	c.SendVersion()

	ver_ack_received := false

main_loop:
	for {
		cmd, er := c.FetchMessage()
		if er != nil {
			println("FetchMessage:", er.Error())
			break
		}

		if cmd!=nil {
			switch cmd.cmd {
				case "version":
					c.VerMsg(cmd.pl)
				
				case "verack":
					fmt.Println("Received Ver ACK")
					ver_ack_received = true
				
				case "inv":
					ProcessInv(cmd.pl)
				
				case "tx": //ParseTx(cmd.pl)
					println("tx unexpected here (now)")
					break main_loop
				
				case "addr":// ParseAddr(cmd.pl)
				
				case "block": //ParseBlock(cmd.pl)
					blockReceived(cmd.pl)

				default:
					println(cmd.cmd)
			}
		}

		if ver_ack_received {
			c.Tick()
		}
	}
	c.TCPConn.Close()
}

func do_network(host string) {
	var conn oneConnection
	oa, e := net.ResolveTCPAddr("tcp4", host)
	if e != nil {
		println(e.Error())
		return
	}
	for {
		conn.TCPConn, e = net.DialTCP("tcp4", nil, oa)
		if e != nil {
			println(e.Error())
			time.Sleep(5e9)
			continue
		}
		println("Connected to bitcoin node", host)

		do_one_connection(conn)
	}
}


