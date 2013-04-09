package main

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"net"
	"os"
	"strings"
	"time"
    "github.com/piotrnar/gocoin/btc"
)

type BCmsg struct {
	cmd string
	pl  []byte
}

func SendRawMsg(conn *net.TCPConn, cmd string, pl []byte) {
	_, e := conn.Write(Magic[:])
	if e != nil {
		println("Conection failed during write 1", e.Error())
		os.Exit(1)
	}

	b := make([]byte, 12)
	copy(b, cmd)
	conn.Write(b[:])

	b[0] = byte(len(pl))
	b[1] = byte(len(pl) >> 8)
	b[2] = byte(len(pl) >> 16)
	b[3] = byte(len(pl) >> 24)
	conn.Write(b[:4])

	s1 := sha256.New()
	s1.Write(pl[:])
	s2 := sha256.New()
	s2.Write(s1.Sum(nil))
	conn.Write(s2.Sum(nil)[:4])

	conn.Write(pl[:])
	//println("sent msg", cmd)
	//dbgmem(pl)
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

func SendVersion(conn *net.TCPConn) {
	b := bytes.NewBuffer([]byte{})

	WriteLSB(b, Version, 4)
	b.Write(Services)
	WriteLSB(b, uint64(time.Now().Unix()), 8)

	putaddr(b, conn.RemoteAddr().String())
	putaddr(b, conn.LocalAddr().String())

	var r [8]byte
	rand.Read(r[:])
	b.Write(r[:])

	b.WriteByte(0)
	WriteLSB(b, 0, 4) // last block

	SendRawMsg(conn, "version", b.Bytes())
}

func FetchMessage(conn *net.TCPConn) (ret BCmsg) {
	var hdr [20]byte
	var cs [4]byte

start_over:
	//conn.SetTimeout(1e9)
	//println("reading response")

	_, e := conn.Read(hdr[:])
ag:
	if e != nil {
		println("FetchMessage error 1", e.Error())
		os.Exit(1)
	}
	if bytes.Compare(hdr[:4], Magic[:]) != 0 {
		println("Proto sync...")
		os.Exit(1)
		copy(hdr[0:19], hdr[1:20])
		_, e = conn.Read(hdr[19:20])
		goto ag
	}

	ret.cmd = strings.TrimRight(string(hdr[4:16]), "\000")
	le := uint32(hdr[16]) | (uint32(hdr[17]) << 8) | (uint32(hdr[18]) << 16) | (uint32(hdr[19]) << 24)
	//println(ret.cmd, le, bin2hex(hdr[:]))

	_, e = conn.Read(cs[:])
	if e != nil {
		println("FetchMessage error 2", e.Error())
		os.Exit(1)
	}

	if le > 0 {
		ret.pl = make([]byte, le)
		var tot, n int
		for tot < int(le) {
			n, e = conn.Read(ret.pl[tot:])
			if e != nil {
				println("FetchMessage payload error", e.Error())
				os.Exit(1)
			}
			tot += n
		}

		s1 := sha256.New()
		s1.Write(ret.pl)

		s2 := sha256.New()
		s2.Write(s1.Sum(nil))

		if bytes.Compare(cs[:], s2.Sum(nil)[:4]) != 0 {
			println("Message", ret.cmd, "checksum error - ignore!", len(ret.pl), bin2hex(cs[:]))
			dbgmem(ret.pl)
			os.Exit(1)
			goto start_over
		}
	}
	return
}


func VerMsg(conn *net.TCPConn, pl []byte) {
	SendRawMsg(conn, "verack", []byte{})
	if *listen {
		SendVersion(conn)
	}
}

func ask4data(conn *net.TCPConn, typ uint32, h [32]byte) {
	b := bytes.NewBuffer([]byte{})
	b.WriteByte(1)
	WriteLSB(b, uint64(typ), 4)
	b.Write(h[:])
	SendRawMsg(conn, "getdata", b.Bytes())
}


func ask4blocks(conn *net.TCPConn) {
	lastbl := BlockChain.BlockTreeEnd
	if lastbl == nil {
		println("ask4blocks: BlockTreeEnd is nil")
		return
	}

	b := bytes.NewBuffer([]byte{})
	WriteLSB(b, Version, 4)
	b.WriteByte(1)
	b.Write(lastbl.BlockHash.Hash[:])
	b.Write(bytes.Repeat([]byte{0}, 32))
	SendRawMsg(conn, "getblocks", b.Bytes())
}


func ProcessInv(conn *net.TCPConn, pl []byte) {
	var h [32]byte
	b := bytes.NewBuffer(pl)
	le := GetVarLen(b)
	var i uint64
	for i = 0; i < le; i++ {
		typ := uint32(ReadLSB(b,4))
		_, e := b.Read(h[:])
		if e != nil {
			println("inv payload broken")
			os.Exit(1)
		}
		
		if dbg>0 {
			fmt.Printf("INV %d: %s ... \n", typ, hash2str(h))
		}

		if (typ==2) {
			// new block?
			ui := btc.NewUint256(h[:])
			_, haveit := BlockChain.BlockIndex[ui.BIdx()]
			if !haveit {
				ask4data(conn, typ, h)
			} else {
				println("INV: alerady have block", ui.String())
			}
		}
	}
	return
}


func do_network(out chan *command) {
	var conn *net.TCPConn
	if !(*listen) {
		oa, e := net.ResolveTCPAddr("tcp4", *host)
		if e != nil {
			println(e.Error())
			return
		}
		conn, e = net.DialTCP("tcp4", nil, oa)
		if e != nil {
			println(e.Error())
			return
		}
		println("Connected to bitcoin node", *host)

		SendVersion(conn)
	} else {
		ad, e := net.ResolveTCPAddr("tcp4", "0.0.0.0:8333");
		if e != nil {
			println(e.Error())
			return
		}

		lis, e := net.ListenTCP("tcp4", ad)
		if e != nil {
			println(e.Error())
			return
		}
		defer lis.Close()

		println("waiting for connection at TCP port 8333...");
		conn, e = lis.AcceptTCP()
		if e != nil {
			println(e.Error())
			return
		}
		println("Incomming connection accepted");
	}

	for {
		c := FetchMessage(conn)

		switch c.cmd {
			case "version":VerMsg(conn, c.pl)
			case "verack":
				fmt.Println("Received Ver ACK")
				ask4blocks(conn)
			case "inv": ProcessInv(conn, c.pl)
			case "tx": //ParseTx(c.pl)
			case "addr":// ParseAddr(c.pl)
			case "block": //ParseBlock(c.pl)
				msg := new(command)
				msg.src = "net"
				msg.str = "bl"
				msg.dat = c.pl
				out <- msg

			default:
				println(c.cmd)
		}
	}
}


