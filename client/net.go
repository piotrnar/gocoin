package main

import (
	"os"
	"fmt"
	"net"
	"time"
	"bytes"
	"errors"
	"strings"
	"crypto/rand"
	"sync/atomic"
	"encoding/binary"
	"github.com/piotrnar/gocoin/btc"
)


const (
	Version = 70001
	UserAgent = "/Gocoin:"+btc.SourcesTag+"/"

	Services = uint64(0x1)

	SendAddrsEvery = (15*time.Minute)
	AskAddrsEvery = (5*time.Minute)

	MaxInCons = 16
	MaxOutCons = 8
	MaxTotCons = MaxInCons+MaxOutCons

	NoDataTimeout = 2*time.Minute

	MaxBytesInSendBuffer = 16*1024 // If we have more than this in the send buffer, we send no more responses

	NewBlocksAskDuration = 30*time.Second  // Ask each conenction for new blocks every 30 min

	GetBlockTimeout = 5*time.Minute  // If you did not get "block" within this time from "getdata", assume it wont come
)


var (
	openCons map[uint64]*oneConnection = make(map[uint64]*oneConnection, MaxTotCons)
	InConsActive, OutConsActive uint
	DefaultTcpPort uint16
	MyExternalAddr *btc.NetAddr
	LastConnId uint32
)


type oneConnection struct {
	// Source of this IP:
	PeerAddr *onePeer
	ConnID uint32

	Broken bool // maker that the conenction has been Broken
	BanIt bool // BanIt this client after disconnecting

	// TCP connection data:
	Incomming bool
	*net.TCPConn

	// Handshake data
	ConnectedAt time.Time
	VerackReceived bool

	// Data from the version message
	node struct {
		version uint32
		services uint64
		timestamp uint64
		height uint32
		agent string
	}

	// Messages reception state machine:
	recv struct {
		hdr [24]byte
		hdr_len int
		dat []byte
		datlen uint32
	}

	// Message sending state machine:
	send struct {
		buf []byte
		sofar int
	}

	// Statistics:
	LoopCnt, TicksCnt uint  // just to see if the threads loop is alive
	BytesReceived, BytesSent uint64
	LastBtsRcvd, LastBtsSent uint32
	LastCmdRcvd, LastCmdSent string

	PendingInvs []*[36]byte // List of pending INV to send and the mutex protecting access to it

	NextAddrSent time.Time // When we shoudl annonce our "addr" again
	NextGetAddr time.Time // When we shoudl issue "getaddr" again

	LastDataGot time.Time // if we have no data for some time, we abort this conenction

	LastBlocksFrom *btc.BlockTreeNode // what the last getblocks was based un
	NextBlocksAsk time.Time           // when the next getblocks should be needed

	GetBlocksInProgress map[[btc.Uint256IdxLen]byte] time.Time // We've sent getdata for a block...
}


func NewConnection(ad *onePeer) (c *oneConnection) {
	c = new(oneConnection)
	c.PeerAddr = ad
	c.GetBlocksInProgress = make(map[[btc.Uint256IdxLen]byte] time.Time)
	c.ConnID = atomic.AddUint32(&LastConnId, 1)
	return
}


func (c *oneConnection) SendRawMsg(cmd string, pl []byte) (e error) {
	if len(c.send.buf) > 1024*1024 {
		println(c.PeerAddr.Ip(), "WTF??", cmd, c.LastCmdSent)
		return
	}

	sbuf := make([]byte, 24+len(pl))

	c.LastCmdSent = cmd
	c.LastBtsSent = uint32(len(pl))

	binary.LittleEndian.PutUint32(sbuf[0:4], Version)
	copy(sbuf[0:4], Magic[:])
	copy(sbuf[4:16], cmd)
	binary.LittleEndian.PutUint32(sbuf[16:20], uint32(len(pl)))

	sh := btc.Sha2Sum(pl[:])
	copy(sbuf[20:24], sh[:4])
	copy(sbuf[24:], pl)

	c.send.buf = append(c.send.buf, sbuf...)

	if dbg<0 {
		fmt.Println(cmd, len(c.send.buf), "->", c.PeerAddr.Ip())
	}
	//println(len(c.send.buf), "queued for seding to", c.PeerAddr.Ip())
	return
}


func (c *oneConnection) DoS() {
	CountSafe("BannedNodes")
	c.BanIt = true
	c.Broken = true
}


func putaddr(b *bytes.Buffer, a string) {
	var ip [4]byte
	var p uint16
	n, e := fmt.Sscanf(a, "%d.%d.%d.%d:%d", &ip[0], &ip[1], &ip[2], &ip[3], &p)
	if e != nil || n != 5 {
		println("Incorrect address:", a)
		os.Exit(1)
	}
	binary.Write(b, binary.LittleEndian, uint64(Services))
	// No Ip6 supported:
	b.Write([]byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0xFF, 0xFF})
	b.Write(ip[:])
	binary.Write(b, binary.BigEndian, uint16(p))
}


func (c *oneConnection) SendVersion() {
	b := bytes.NewBuffer([]byte{})

	binary.Write(b, binary.LittleEndian, uint32(Version))
	binary.Write(b, binary.LittleEndian, uint64(Services))
	binary.Write(b, binary.LittleEndian, uint64(time.Now().Unix()))

	putaddr(b, c.TCPConn.RemoteAddr().String())
	putaddr(b, c.TCPConn.LocalAddr().String())

	var nonce [8]byte
	rand.Read(nonce[:])
	b.Write(nonce[:])

	b.WriteByte(byte(len(UserAgent)))
	b.Write([]byte(UserAgent))

	binary.Write(b, binary.LittleEndian, uint32(LastBlock.Height))
	b.WriteByte(0)  // don't notify me about txs

	c.SendRawMsg("version", b.Bytes())
}


func (c *oneConnection) HandleError(e error) (error) {
	if nerr, ok := e.(net.Error); ok && nerr.Timeout() {
		//fmt.Println("Just a timeout - ignore")
		return nil
	}
	if dbg>0 {
		println("HandleError:", e.Error())
	}
	c.recv.hdr_len = 0
	c.recv.dat = nil
	c.Broken = true
	return e
}


type BCmsg struct {
	cmd string
	pl  []byte
}

func (c *oneConnection) FetchMessage() (*BCmsg) {
	var e error
	var n int

	for c.recv.hdr_len < 24 {
		n, e = SockRead(c.TCPConn, c.recv.hdr[c.recv.hdr_len:24])
		c.recv.hdr_len += n
		if e != nil {
			c.HandleError(e)
			return nil
		}
		if c.recv.hdr_len>=4 && !bytes.Equal(c.recv.hdr[:4], Magic[:]) {
			println("FetchMessage: Proto out of sync")
			c.Broken = true
			return nil
		}
		if c.Broken {
			return nil
		}
	}

	dlen :=  binary.LittleEndian.Uint32(c.recv.hdr[16:20])
	if dlen > 0 {
		if c.recv.dat == nil {
			c.recv.dat = make([]byte, dlen)
			c.recv.datlen = 0
		}
		for c.recv.datlen < dlen {
			n, e = SockRead(c.TCPConn, c.recv.dat[c.recv.datlen:])
			c.recv.datlen += uint32(n)
			if e != nil {
				c.HandleError(e)
				return nil
			}
			if c.Broken {
				return nil
			}
		}
	}

	sh := btc.Sha2Sum(c.recv.dat)
	if !bytes.Equal(c.recv.hdr[20:24], sh[:4]) {
		if dbg > 0 {
			println(c.PeerAddr.Ip(), "Msg checksum error")
		}
		CountSafe("BadMsgChecksum")
		c.DoS()
		c.recv.hdr_len = 0
		c.recv.dat = nil
		c.Broken = true
		return nil
	}

	ret := new(BCmsg)
	ret.cmd = strings.TrimRight(string(c.recv.hdr[4:16]), "\000")
	ret.pl = c.recv.dat
	c.recv.dat = nil
	c.recv.hdr_len = 0

	c.BytesReceived += uint64(24+len(ret.pl))

	return ret
}


func (c *oneConnection) SendAddr() {
	buf := new(bytes.Buffer)
	mutex.Lock()
	adrscnt := uint32(len(openCons))
	sendown := !c.NextAddrSent.IsZero() && MyExternalAddr!=nil
	if sendown {
		adrscnt++
	}
	if adrscnt > 0 {
		now := uint32(time.Now().Unix())

		btc.WriteVlen(buf, adrscnt)

		for _, v := range openCons {
			binary.Write(buf, binary.LittleEndian, now)
			tmp := v.PeerAddr.NetAddr.Bytes()
			buf.Write(tmp[:])
		}

		if sendown {
			binary.Write(buf, binary.LittleEndian, now)
			tmp := MyExternalAddr.Bytes()
			buf.Write(tmp[:])
		}
	}
	// Store our own address
	mutex.Unlock()
	if !c.NextAddrSent.IsZero() {
		c.NextAddrSent = time.Now().Add(SendAddrsEvery)
	}

	if adrscnt > 0 {
		c.SendRawMsg("addr", buf.Bytes())
	}
}


func (c *oneConnection) HandleVersion(pl []byte) error {
	if len(pl) >= 46 {
		c.node.version = binary.LittleEndian.Uint32(pl[0:4])
		c.node.services = binary.LittleEndian.Uint64(pl[4:12])
		c.node.timestamp = binary.LittleEndian.Uint64(pl[12:20])
		if MyExternalAddr == nil {
			MyExternalAddr = btc.NewNetAddr(pl[20:46]) // These bytes should know our external IP
			MyExternalAddr.Port = DefaultTcpPort
		}
		if len(pl) >= 86 {
			le, of := btc.VLen(pl[80:])
			of += 80
			c.node.agent = string(pl[of:of+le])
			of += le
			if len(pl) >= of+4 {
				c.node.height = binary.LittleEndian.Uint32(pl[of:of+4])
			}
		}
	} else {
		return errors.New("Version message too short")
	}
	c.SendRawMsg("verack", []byte{})
	return nil
}


func (c *oneConnection) GetBlocks(lastbl []byte) {
	if dbg > 0 {
		println(c.PeerAddr.Ip(), "getblocks since", btc.NewUint256(lastbl).String())
	}
	var b [4+1+32+32]byte
	binary.LittleEndian.PutUint32(b[0:4], Version)
	b[4] = 1 // only one locator
	copy(b[5:37], lastbl)
	// the remaining bytes should be filled with zero
	c.SendRawMsg("getblocks", b[:])
}


func (c *oneConnection) ProcessInv(pl []byte) {
	if len(pl) < 37 {
		println(c.PeerAddr.Ip(), "inv payload too short", len(pl))
		return
	}

	cnt, of := btc.VLen(pl)
	if len(pl) != of + 36*cnt {
		println("inv payload length mismatch", len(pl), of, cnt)
	}

	for i:=0; i<cnt; i++ {
		typ := binary.LittleEndian.Uint32(pl[of:of+4])
		if typ==2 {
			InvsNotify(pl[of+4:of+36])
			/*if cnt>100 && i==cnt-1 {
				c.GetBlocks(pl[of+4:of+36])
			}*/
		} else if typ==1 {
			CountSafe("InvGotTxs")
		} else {
			CountSafe("InvGot???")
		}
		of+= 36
	}
	return
}


// This function is called from the main thread
func NetSendInv(typ uint32, h []byte, fromConn *oneConnection) (cnt uint) {
	// Prepare the inv
	inv := new([36]byte)
	binary.LittleEndian.PutUint32(inv[0:4], typ)
	copy(inv[4:36], h)

	// Append it to PendingInvs in each open connection
	mutex.Lock()
	for _, v := range openCons {
		if v != fromConn { // except for the one that this inv came from
			if len(v.PendingInvs)<500 {
				v.PendingInvs = append(v.PendingInvs, inv)
				cnt++
			} else {
				CountSafe("SendInvIgnored")
			}
		}
	}
	mutex.Unlock()
	return
}


func addInvBlockBranch(inv map[[32]byte] bool, bl *btc.BlockTreeNode, stop *btc.Uint256) {
	if len(inv)>=500 || bl.BlockHash.Equal(stop) {
		return
	}
	inv[bl.BlockHash.Hash] = true
	for i := range bl.Childs {
		if len(inv)>=500 {
			return
		}
		addInvBlockBranch(inv, bl.Childs[i], stop)
	}
}


func (c *oneConnection) ProcessGetBlocks(pl []byte) {
	b := bytes.NewReader(pl)
	var ver uint32
	e := binary.Read(b, binary.LittleEndian, &ver)
	if e != nil {
		println("ProcessGetBlocks:", e.Error(), c.PeerAddr.Ip())
		c.DoS()
		return
	}
	cnt, e := btc.ReadVLen(b)
	if e != nil {
		println("ProcessGetBlocks:", e.Error(), c.PeerAddr.Ip())
		c.DoS()
		return
	}
	h2get := make([]*btc.Uint256, cnt)
	var h [32]byte
	for i:=0; i<int(cnt); i++ {
		n, _ := b.Read(h[:])
		if n != 32 {
			println("getblocks too short", c.PeerAddr.Ip())
			CountSafe("GetBlocksTooShort")
			c.DoS()
			return
		}
		h2get[i] = btc.NewUint256(h[:])
		if dbg>2 {
			println(c.PeerAddr.Ip(), "getbl", h2get[i].String())
		}
	}
	n, _ := b.Read(h[:])
	if n != 32 {
		println("getblocks does not have hash_stop", c.PeerAddr.Ip())
		CountSafe("GetBlocksNoStop")
		c.DoS()
		return
	}
	hashstop := btc.NewUint256(h[:])

	var maxheight, minheight uint32
	minheight = 0xffffffff
	invs := make(map[[32]byte] bool, 500)
	for i := range h2get {
		BlockChain.BlockIndexAccess.Lock()
		if bl, ok := BlockChain.BlockIndex[h2get[i].BIdx()]; ok {
			if bl.Height<minheight {
				minheight = bl.Height
			}
			if bl.Height > maxheight {
				maxheight = bl.Height
			}
			addInvBlockBranch(invs, bl, hashstop)
		}
		BlockChain.BlockIndexAccess.Unlock()
		if len(invs)>=500 {
			break
		}
	}
	if len(invs) > 0 {
		inv := new(bytes.Buffer)
		btc.WriteVlen(inv, uint32(len(invs)))
		for k, _ := range invs {
			binary.Write(inv, binary.LittleEndian, uint32(2))
			inv.Write(k[:])
		}
		if dbg>0 {
			fmt.Printf("%s: getblocks  cnt=%d  h:%d..%d => %d invs / %d bytes\n",
				c.PeerAddr.Ip(), cnt, minheight, maxheight, len(invs), len(inv.Bytes()))
		}
		CountSafe("GetblocksReplies")
		c.SendRawMsg("inv", inv.Bytes())
	}
}


func (c *oneConnection) ProcessGetData(pl []byte) {
	//println(c.PeerAddr.Ip(), "getdata")
	b := bytes.NewReader(pl)
	cnt, e := btc.ReadVLen(b)
	if e != nil {
		println("ProcessGetData:", e.Error(), c.PeerAddr.Ip())
		return
	}
	for i:=0; i<int(cnt); i++ {
		var typ uint32
		var h [32]byte

		e = binary.Read(b, binary.LittleEndian, &typ)
		if e != nil {
			println("ProcessGetData:", e.Error(), c.PeerAddr.Ip())
			return
		}

		n, _ := b.Read(h[:])
		if n!=32 {
			println("ProcessGetData: pl too short", c.PeerAddr.Ip())
			return
		}

		if typ == 2 {
			uh := btc.NewUint256(h[:])
			bl, _, er := BlockChain.Blocks.BlockGet(uh)
			if er == nil {
				CountSafe("BlockSent")
				c.SendRawMsg("block", bl)
			} else {
				//println("block", uh.String(), er.Error())
			}
		} else if typ == 1 {
			// transaction
			uh := btc.NewUint256(h[:])
			if tx, ok := TransactionsToSend[uh.Hash]; ok {
				c.SendRawMsg("tx", tx)
				CountSafe("TxsSent")
				if dbg > 0 {
					println("sent tx to", c.PeerAddr.Ip())
				}
			}
		} else {
			println("getdata for type", typ, "not supported yet")
		}

		if len(c.send.buf) >= MaxBytesInSendBuffer {
			if dbg > 0 {
				println(c.PeerAddr.Ip(), "Too many bytes")
			}
			break
		}
	}
}


func (c *oneConnection) GetBlockData(h []byte) {
	var b [1+4+32]byte
	b[0] = 1 // One inv
	b[1] = 2 // Block
	copy(b[5:37], h[:32])
	if dbg > 1 {
		println("GetBlockData", btc.NewUint256(h).String())
	}
	c.SendRawMsg("getdata", b[:])
}


func (c *oneConnection) SendInvs() (res bool) {
	b := new(bytes.Buffer)
	mutex.Lock()
	if len(c.PendingInvs)>0 {
		btc.WriteVlen(b, uint32(len(c.PendingInvs)))
		for i := range c.PendingInvs {
			b.Write((*c.PendingInvs[i])[:])
		}
		res = true
	}
	c.PendingInvs = nil
	mutex.Unlock()
	if res {
		c.SendRawMsg("inv", b.Bytes())
	}
	return
}


func (c *oneConnection) blocksNeeded() bool {
	mutex.Lock()
	force := c.LastBlocksFrom != LastBlock
	mutex.Unlock()
	if force || time.Now().After(c.NextBlocksAsk) {
		c.LastBlocksFrom = LastBlock

		// Lock the blocktree while we're browsing through it
		// ask N-blocks up in the chain, to recover from dead-end chain forks

		BlockChain.BlockIndexAccess.Lock()
		// Look one block deeper, with each lasting minute since the last block was received
		depth := int(time.Now().Sub(LastBlockReceived)/time.Minute)
		if depth>200 { // ... but don't get too crazy with it
			depth = 200
		}
		n := LastBlock
		for i:=0; i<depth && n.Parent != nil; i++ {
			n = n.Parent
		}
		BlockChain.BlockIndexAccess.Unlock()

		CountSafe("GetblocksRequested")
		c.GetBlocks(n.BlockHash.Hash[:])
		c.NextBlocksAsk = time.Now().Add(NewBlocksAskDuration)
		return true
	}
	return false
}


func (c *oneConnection) Tick() {
	c.TicksCnt++

	// Check no-data timeout
	if c.LastDataGot.Add(NoDataTimeout).Before(time.Now()) {
		c.Broken = true
		CountSafe("NetConnTimeouts")
		if dbg>0 {
			println(c.PeerAddr.Ip(), "no data for", NoDataTimeout, "seconds - disconnect")
		}
		return
	}

	if c.send.buf != nil {
		max2send := len(c.send.buf) - c.send.sofar
		if max2send > 4096 {
			max2send = 4096
		}
		n, e := SockWrite(c.TCPConn, c.send.buf[c.send.sofar:])
		if n > 0 {
			c.
			LastDataGot = time.Now()
			c.BytesSent += uint64(n)
			c.send.sofar += n
			//println(c.PeerAddr.Ip(), max2send, "...", c.send.sofar, n, e)
			if c.send.sofar >= len(c.send.buf) {
				c.send.buf = nil
				c.send.sofar = 0
			}
		}
		if e != nil {
			if dbg > 0 {
				println(c.PeerAddr.Ip(), "Connection Broken during send")
			}
			c.Broken = true
		}
		return
	}

	if !c.VerackReceived {
		// If we have no ack, do nothing more.
		return
	}

	// Need to send some invs...?
	if c.SendInvs() {
		return
	}

	// Need to send getdata...?
	if tmp := blockDataNeeded(); tmp != nil {
		idx := btc.NewUint256(tmp).BIdx()
		if t, pr := c.GetBlocksInProgress[idx]; !pr || time.Now().After(t.Add(GetBlockTimeout)) {
			c.GetBlockData(tmp)
			c.GetBlocksInProgress[idx] = time.Now()
		} else {
			CountSafe("GetBlocksInProgress")
		}
		return
	}

	// Need to send getblocks...?
	if c.blocksNeeded() {
		return
	}

	// Ask node for new addresses...?
	if time.Now().After(c.NextGetAddr) {
		CountSafe("GetaddrSent")
		c.SendRawMsg("getaddr", nil)
		c.NextGetAddr = time.Now().Add(AskAddrsEvery)
		return
	}

	// Announce our own address...?
	if !c.NextAddrSent.IsZero() && time.Now().After(c.NextAddrSent) {
		c.SendAddr()
		return
	}
}


func do_one_connection(c *oneConnection) {
	if !c.Incomming {
		c.SendVersion()
	}

	c.LastDataGot = time.Now()
	c.NextBlocksAsk = time.Now() // askf ro blocks ASAP
	c.NextGetAddr = time.Now().Add(10*time.Second)  // do getaddr ~10 seconds from now
	if *server {
		c.NextAddrSent = c.NextGetAddr // If not a server, send "addr" only when asked
	}

	for !c.Broken {
		c.LoopCnt++
		cmd := c.FetchMessage()
		if c.Broken {
			break
		}

		if cmd==nil {
			c.Tick()
			continue
		}

		c.LastDataGot = time.Now()
		c.LastCmdRcvd = cmd.cmd
		c.LastBtsRcvd = uint32(len(cmd.pl))

		c.PeerAddr.Alive()
		if dbg<0 {
			fmt.Println(c.PeerAddr.Ip(), "->", cmd.cmd, len(cmd.pl))
		}

		switch cmd.cmd {
			case "version":
				er := c.HandleVersion(cmd.pl)
				if er != nil {
					println("version:", er.Error())
					c.Broken = true
				} else if c.Incomming {
					c.SendVersion()
				}

			case "verack":
				c.VerackReceived = true

			case "inv":
				c.ProcessInv(cmd.pl)

			case "tx": //ParseTx(cmd.pl)
				println("tx unexpected here (now)")
				//c.Broken = true

			case "addr":
				ParseAddr(cmd.pl)

			case "block": //block received
				netBlockReceived(c, cmd.pl)

			case "getblocks":
				if len(c.send.buf) < MaxBytesInSendBuffer {
					c.ProcessGetBlocks(cmd.pl)
				} else {
					CountSafe("CmdGetblocksIgnored")
				}

			case "getdata":
				if len(c.send.buf) < MaxBytesInSendBuffer {
					c.ProcessGetData(cmd.pl)
				} else {
					CountSafe("CmdGetdataIgnored")
				}

			case "getaddr":
				if len(c.send.buf) < MaxBytesInSendBuffer {
					c.SendAddr()
				} else {
					CountSafe("CmdGetaddrIgnored")
				}

			case "alert":
				c.HandleAlert(cmd.pl)

			case "ping":
				if len(c.send.buf) < MaxBytesInSendBuffer {
					re := make([]byte, len(cmd.pl))
					copy(re, cmd.pl)
					c.SendRawMsg("pong", re)
				} else {
					CountSafe("CmdPingIgnored")
				}

			case "notfound":
				CountSafe("NotFound")

			default:
				println(cmd.cmd, "from", c.PeerAddr.Ip())
		}
	}
	if c.BanIt {
		c.PeerAddr.Ban()
	}
	if dbg>0 {
		println("Disconnected from", c.PeerAddr.Ip())
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
	conn := NewConnection(ad)
	mutex.Lock()
	if _, ok := openCons[ad.UniqID()]; ok {
		if dbg>0 {
			fmt.Println(ad.Ip(), "already connected")
		}
		CountSafe("ConnectingAgain")
		mutex.Unlock()
		return
	}
	openCons[ad.UniqID()] = conn
	OutConsActive++
	mutex.Unlock()
	go func() {
		conn.TCPConn, e = net.DialTCP("tcp4", nil, &net.TCPAddr{
			IP: net.IPv4(ad.Ip4[0], ad.Ip4[1], ad.Ip4[2], ad.Ip4[3]),
			Port: int(ad.Port)})
		if e == nil {
			conn.ConnectedAt = time.Now()
			if dbg>0 {
				println("Connected to", ad.Ip())
			}
			do_one_connection(conn)
		} else {
			if dbg>0 {
				println("Could not connect to", ad.Ip())
			}
			//println(e.Error())
		}
		mutex.Lock()
		delete(openCons, ad.UniqID())
		OutConsActive--
		mutex.Unlock()
		ad.Dead()
	}()
}


func network_process() {
	if *server {
		go start_server()
	}
	for {
		mutex.Lock()
		conn_cnt := OutConsActive
		mutex.Unlock()
		if conn_cnt < MaxOutCons {
			ad := getBestPeer()
			if ad != nil {
				do_network(ad)
			} else if *proxy=="" && dbg>0 {
				println("no new peers", len(openCons), conn_cnt)
			}
		}
		time.Sleep(250e6)
	}
}


func start_server() {
	ad, e := net.ResolveTCPAddr("tcp4", fmt.Sprint("0.0.0.0:", DefaultTcpPort))
	if e != nil {
		println("ResolveTCPAddr", e.Error())
		return
	}

	lis, e := net.ListenTCP("tcp4", ad)
	if e != nil {
		println("ListenTCP", e.Error())
		return
	}
	defer lis.Close()

	fmt.Println("TCP server started at", ad.String())

	for {
		if InConsActive < MaxInCons {
			tc, e := lis.AcceptTCP()
			if e == nil {
				if dbg>0 {
					fmt.Println("Incomming connection from", tc.RemoteAddr().String())
				}
				ad := newIncommingPeer(tc.RemoteAddr().String())
				if ad != nil {
					conn := NewConnection(ad)
					conn.ConnectedAt = time.Now()
					conn.Incomming = true
					conn.TCPConn = tc
					mutex.Lock()
					if _, ok := openCons[ad.UniqID()]; ok {
						//fmt.Println(ad.Ip(), "already connected")
						CountSafe("SameIpReconnect")
						mutex.Unlock()
					} else {
						openCons[ad.UniqID()] = conn
						InConsActive++
						mutex.Unlock()
						go func () {
							do_one_connection(conn)
							mutex.Lock()
							delete(openCons, ad.UniqID())
							InConsActive--
							mutex.Unlock()
						}()
					}
				} else {
					println("newIncommingPeer failed - IP probably baned", tc.RemoteAddr().String())
					CountSafe("InConnRefused")
					tc.Close()
				}
			}
		} else {
			time.Sleep(250e6)
		}
	}
}
