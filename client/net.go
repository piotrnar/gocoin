package main

import (
	"fmt"
	"net"
	"time"
	"sync"
	"sort"
	"bytes"
	"errors"
	"strings"
	mr "math/rand"
	"sync/atomic"
	"crypto/rand"
	"encoding/binary"
	"github.com/piotrnar/gocoin/btc"
)


const (
	Version = 70001
	UserAgent = "/Gocoin:"+btc.SourcesTag+"/"

	Services = uint64(0x1)

	AskAddrsEvery = (5*time.Minute)
	MaxAddrsPerMessage = 500

	MaxInCons = 16
	MaxOutCons = 9
	MaxTotCons = MaxInCons+MaxOutCons

	NoDataTimeout = 2*time.Minute

	MaxBytesInSendBuffer = 16*1024 // If we have more than this bytes in the send buffer, send no more responses

	NewBlocksAskDuration = 5*time.Minute  // Ask each connection for new blocks every X minutes

	GetBlockTimeout = 1*time.Minute  // If you did not get "block" within this time from "getdata", assume it won't come

	TCPDialTimeout = 10*time.Second // If it does not connect within this time, assume it dead
	AnySendTimeout = 30*time.Second // If it does not send a byte within this time, assume it dead

	PingPeriod = 60*time.Second
	PingTimeout = 5*time.Second
	PingHistoryLength = 8
	PingHistoryValid = (PingHistoryLength-4) // Ignore N longest pings
	PingAssumedIfUnsupported = 999 // ms

	DropSlowestEvery = 10*time.Minute // Look for the slowest peer and drop it

	MIN_PROTO_VERSION = 209
)


var (
	openCons map[uint64]*oneConnection = make(map[uint64]*oneConnection, MaxTotCons)
	InConsActive, OutConsActive uint
	DefaultTcpPort uint16
	ExternalIp4 map[uint32]uint = make(map[uint32]uint)
	ExternalIpMutex sync.Mutex
	LastConnId uint32
	nonce [8]byte
)


type oneConnection struct {
	// Source of this IP:
	PeerAddr *onePeer
	ConnID uint32

	Broken bool // maker that the conenction has been Broken
	BanIt bool // BanIt this client after disconnecting

	// TCP connection data:
	Incomming bool
	NetConn net.Conn

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
		lastSent time.Time
	}

	// Statistics:
	LoopCnt, TicksCnt uint  // just to see if the threads loop is alive
	BytesReceived, BytesSent uint64
	LastBtsRcvd, LastBtsSent uint32
	LastCmdRcvd, LastCmdSent string

	PendingInvs []*[36]byte // List of pending INV to send and the mutex protecting access to it

	NextGetAddr time.Time // When we shoudl issue "getaddr" again

	LastDataGot time.Time // if we have no data for some time, we abort this conenction

	LastBlocksFrom *btc.BlockTreeNode // what the last getblocks was based un
	NextBlocksAsk time.Time           // when the next getblocks should be needed

	GetBlockInProgress *btc.Uint256
	GetBlockInProgressAt time.Time

	// Ping stats
	PingHistory [PingHistoryLength]int
	PingHistoryIdx int
	NextPing time.Time
	PingInProgress []byte
	LastPingSent time.Time
}


func init() {
	rand.Read(nonce[:])
}


func NewConnection(ad *onePeer) (c *oneConnection) {
	c = new(oneConnection)
	c.PeerAddr = ad
	c.ConnID = atomic.AddUint32(&LastConnId, 1)
	return
}


func (c *oneConnection) SendRawMsg(cmd string, pl []byte) (e error) {
	if len(c.send.buf) > 1024*1024 {
		println(c.PeerAddr.Ip(), "WTF??", cmd, c.LastCmdSent)
		return
	}

	CountSafe("sent_"+cmd)
	CountSafeAdd("sbts_"+cmd, uint64(len(pl)))
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
	c.send.lastSent = time.Now()

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


func ExternalAddrLen() (res int) {
	ExternalIpMutex.Lock()
	res = len(ExternalIp4)
	ExternalIpMutex.Unlock()
	return
}


func BestExternalAddr() []byte {
	var best_ip uint32
	var best_cnt uint
	ExternalIpMutex.Lock()
	for ip, cnt := range ExternalIp4 {
		if cnt > best_cnt {
			best_cnt = cnt
			best_ip = ip
		}
	}
	ExternalIpMutex.Unlock()
	res := make([]byte, 26)
	binary.LittleEndian.PutUint64(res[0:8], Services)
	// leave ip6 filled with zeros, except for the last 2 bytes:
	res[18], res[19] = 0xff, 0xff
	binary.BigEndian.PutUint32(res[20:24], best_ip)
	binary.BigEndian.PutUint16(res[24:26], DefaultTcpPort)
	return res
}


func (c *oneConnection) SendVersion() {
	b := bytes.NewBuffer([]byte{})

	binary.Write(b, binary.LittleEndian, uint32(Version))
	binary.Write(b, binary.LittleEndian, uint64(Services))
	binary.Write(b, binary.LittleEndian, uint64(time.Now().Unix()))

	b.Write(c.PeerAddr.NetAddr.Bytes())
	if ExternalAddrLen()>0 {
		b.Write(BestExternalAddr())
	} else {
		b.Write(bytes.Repeat([]byte{0}, 26))
	}

	b.Write(nonce[:])

	b.WriteByte(byte(len(UserAgent)))
	b.Write([]byte(UserAgent))

	binary.Write(b, binary.LittleEndian, uint32(LastBlock.Height))
	if !*txrounting {
		b.WriteByte(0)  // don't notify me about txs
	}

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
		n, e = SockRead(c.NetConn, c.recv.hdr[c.recv.hdr_len:24])
		c.recv.hdr_len += n
		if e != nil {
			c.HandleError(e)
			return nil
		}
		if c.recv.hdr_len>=4 && !bytes.Equal(c.recv.hdr[:4], Magic[:]) {
			if dbg >0 {
				println("FetchMessage: Proto out of sync")
			}
			CountSafe("NetBadMagic")
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
			n, e = SockRead(c.NetConn, c.recv.dat[c.recv.datlen:])
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
		CountSafe("NetBadChksum")
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
	pers := GetBestPeers(MaxAddrsPerMessage, false)
	if len(pers)>0 {
		buf := new(bytes.Buffer)
		btc.WriteVlen(buf, uint32(len(pers)))
		for i := range pers {
			binary.Write(buf, binary.LittleEndian, pers[i].Time)
			buf.Write(pers[i].NetAddr.Bytes())
		}
		c.SendRawMsg("addr", buf.Bytes())
	}
}


func (c *oneConnection) SendOwnAddr() {
	if ExternalAddrLen()>0 {
		buf := new(bytes.Buffer)
		btc.WriteVlen(buf, 1)
		binary.Write(buf, binary.LittleEndian, uint32(time.Now().Unix()))
		buf.Write(BestExternalAddr())
		c.SendRawMsg("addr", buf.Bytes())
	}
}


func (c *oneConnection) HandleVersion(pl []byte) error {
	if len(pl) >= 80 /*Up to, includiong, the nonce */ {
		c.node.version = binary.LittleEndian.Uint32(pl[0:4])
		if bytes.Equal(pl[72:80], nonce[:]) {
			return errors.New("Connecting to ourselves")
		}
		if c.node.version < MIN_PROTO_VERSION {
			return errors.New("Client version too low")
		}
		c.node.services = binary.LittleEndian.Uint64(pl[4:12])
		c.node.timestamp = binary.LittleEndian.Uint64(pl[12:20])
		if ValidIp4(pl[40:44]) {
			ExternalIpMutex.Lock()
			ExternalIp4[binary.BigEndian.Uint32(pl[40:44])]++
			ExternalIpMutex.Unlock()
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


func (c *oneConnection) ProcessInv(pl []byte) {
	if len(pl) < 37 {
		println(c.PeerAddr.Ip(), "inv payload too short", len(pl))
		return
	}

	cnt, of := btc.VLen(pl)
	if len(pl) != of + 36*cnt {
		println("inv payload length mismatch", len(pl), of, cnt)
	}

	var new_block bool
	var last_inv []byte
	for i:=0; i<cnt; i++ {
		typ := binary.LittleEndian.Uint32(pl[of:of+4])
		CountSafe(fmt.Sprint("InvGot",typ))
		if typ==2 {
			last_inv = pl[of+4:of+36]
			new_block = BlockInvNotify(last_inv)
		} else if typ==1 {
			if *txrounting {
				c.TxInvNotify(pl[of+4:of+36])
			}
		}
		of+= 36
	}

	if cnt==1 && new_block {
		// If this was a single inv for 1 unknown block, ask for it immediately
		if c.GetBlockInProgress==nil {
			CountSafe("InvNewBlockNow")
			c.GetBlockInProgress = btc.NewUint256(last_inv)
			c.GetBlockInProgressAt = time.Now()
			c.GetBlockData(last_inv)
		} else {
			CountSafe("InvNewBlockBusy")
		}
	}
	return
}


// This function is called from the main thread (or from an UI)
func NetRouteInv(typ uint32, h *btc.Uint256, fromConn *oneConnection) (cnt uint) {
	CountSafe(fmt.Sprint("NetRouteInv", typ))

	// Prepare the inv
	inv := new([36]byte)
	binary.LittleEndian.PutUint32(inv[0:4], typ)
	copy(inv[4:36], h.Bytes())

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


// Call this function only when BlockIndexAccess is locked
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

	if cnt<1 {
		println("ProcessGetBlocks: empty inv list", c.PeerAddr.Ip())
		c.DoS()
		return
	}

	h2get := make([]*btc.Uint256, cnt)
	var h [32]byte
	for i:=0; i<int(cnt); i++ {
		n, _ := b.Read(h[:])
		if n != 32 {
			println("getblocks too short", c.PeerAddr.Ip())
			CountSafe("GetblksShort")
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
		CountSafe("GetblksNoStop")
		c.DoS()
		return
	}
	hashstop := btc.NewUint256(h[:])

	invs := make(map[[32]byte] bool, 500)
	for i := range h2get {
		BlockChain.BlockIndexAccess.Lock()
		if bl, ok := BlockChain.BlockIndex[h2get[i].BIdx()]; ok {
			// make sure that this block is in our main chain
			for end := LastBlock; end!=nil && end.Height>=bl.Height; end = end.Parent {
				if end==bl {
					addInvBlockBranch(invs, bl, hashstop)  // Yes - this is the main chain
					if dbg>0 {
						fmt.Println(c.PeerAddr.Ip(), "getblocks from", bl.Height,
							"stop at",  hashstop.String(), "->", len(invs), "invs")
					}

					if len(invs)>0 {
						BlockChain.BlockIndexAccess.Unlock()

						inv := new(bytes.Buffer)
						btc.WriteVlen(inv, uint32(len(invs)))
						for k, _ := range invs {
							binary.Write(inv, binary.LittleEndian, uint32(2))
							inv.Write(k[:])
						}
						c.SendRawMsg("inv", inv.Bytes())
						return
					}
				}
			}
		}
		BlockChain.BlockIndexAccess.Unlock()
	}

	CountSafe("GetblksMissed")
	return
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
				c.SendRawMsg("block", bl)
			} else {
				//println("block", uh.String(), er.Error())
			}
		} else if typ == 1 {
			// transaction
			uh := btc.NewUint256(h[:])
			tx_mutex.Lock()
			if tx, ok := TransactionsToSend[uh.Hash]; ok {
				tx_mutex.Unlock()
				c.SendRawMsg("tx", tx.data)
				CountSafe("TxsSent")
				if dbg > 0 {
					println("sent tx to", c.PeerAddr.Ip())
				}
			} else {
				tx_mutex.Unlock()
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


func (c *oneConnection) getblocksNeeded() bool {
	mutex.Lock()
	lb := LastBlock
	mutex.Unlock()
	if lb != c.LastBlocksFrom || time.Now().After(c.NextBlocksAsk) {
		c.LastBlocksFrom = LastBlock

		GetBlocksAskBack := int(time.Now().Sub(LastBlockReceived) / time.Minute)
		if GetBlocksAskBack >= btc.MovingCheckopintDepth {
			GetBlocksAskBack = btc.MovingCheckopintDepth
		}

		b := make([]byte, 37)
		binary.LittleEndian.PutUint32(b[0:4], Version)
		b[4] = 1 // one locator so far...
		copy(b[5:37], LastBlock.BlockHash.Hash[:])

		if GetBlocksAskBack > 0 {
			BlockChain.BlockIndexAccess.Lock()
			cnt_each := 0
			for i:=0; i < GetBlocksAskBack && lb.Parent != nil; i++ {
				lb = lb.Parent
				cnt_each++
				if cnt_each==200 {
					b[4]++
					b = append(b, lb.BlockHash.Hash[:]...)
					cnt_each = 0
				}
			}
			if cnt_each!=0 {
				b[4]++
				b = append(b, lb.BlockHash.Hash[:]...)
			}
			BlockChain.BlockIndexAccess.Unlock()
		}
		var null_stop [32]byte
		b = append(b, null_stop[:]...)
		c.SendRawMsg("getblocks", b)
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
		CountSafe("NetNodataTout")
		if dbg>0 {
			println(c.PeerAddr.Ip(), "no data for", NoDataTimeout/time.Second, "seconds - disconnect")
		}
		return
	}

	if c.send.buf != nil {
		max2send := len(c.send.buf) - c.send.sofar
		if max2send > 4096 {
			max2send = 4096
		}
		n, e := SockWrite(c.NetConn, c.send.buf[c.send.sofar:])
		if n > 0 {
			c.send.lastSent = time.Now()
			c.BytesSent += uint64(n)
			c.send.sofar += n
			//println(c.PeerAddr.Ip(), max2send, "...", c.send.sofar, n, e)
			if c.send.sofar >= len(c.send.buf) {
				c.send.buf = nil
				c.send.sofar = 0
			}
		} else if time.Now().After(c.send.lastSent.Add(AnySendTimeout)) {
			CountSafe("PeerSendTimeout")
			c.Broken = true
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

	if c.GetBlockInProgress!=nil && time.Now().After(c.GetBlockInProgressAt.Add(GetBlockTimeout)) {
		CountSafe("GetBlockTimeout")
		c.GetBlockInProgress = nil
	}

	// Need to send getdata...?
	if c.GetBlockInProgress==nil {
		if tmp := blockDataNeeded(); tmp != nil {
			c.GetBlockInProgress = btc.NewUint256(tmp)
			c.GetBlockInProgressAt = time.Now()
			c.GetBlockData(tmp)
			return
		}
	}

	// Need to send getblocks...?
	if c.getblocksNeeded() {
		return
	}

	// Ask node for new addresses...?
	if time.Now().After(c.NextGetAddr) {
		if peerDB.Count() > MaxPeersNeeded {
			// If we have a lot of peers, do not ask for more, to save bandwidth
			CountSafe("AddrEnough")
		} else {
			CountSafe("AddrWanted")
			c.SendRawMsg("getaddr", nil)
		}
		c.NextGetAddr = time.Now().Add(AskAddrsEvery)
		return
	}

	// Ping if we dont do anything
	if c.node.version>60000 && c.PingInProgress == nil && time.Now().After(c.NextPing) {
		/*&&len(c.send.buf)==0 && len(c.GetBlocksInProgress)==0*/
		c.PingInProgress = make([]byte, 8)
		rand.Read(c.PingInProgress[:])
		c.SendRawMsg("ping", c.PingInProgress)
		c.LastPingSent = time.Now()
		//println(c.PeerAddr.Ip(), "ping...")
		return
	}
}


func (c *oneConnection) HandlePong() {
	ms := time.Now().Sub(c.LastPingSent) / time.Millisecond
	if dbg>1 {
		println(c.PeerAddr.Ip(), "pong after", ms, "ms", time.Now().Sub(c.LastPingSent).String())
	}
	c.PingHistory[c.PingHistoryIdx] = int(ms)
	c.PingHistoryIdx = (c.PingHistoryIdx+1)%PingHistoryLength
	c.PingInProgress = nil
	c.NextPing = time.Now().Add(PingPeriod)
}


func (c *oneConnection) GetAveragePing() int {
	if c.node.version>60000 {
		var pgs[PingHistoryLength] int
		copy(pgs[:], c.PingHistory[:])
		sort.Ints(pgs[:])
		var sum int
		for i:=0; i<PingHistoryValid; i++ {
			sum += pgs[i]
		}
		return sum/PingHistoryValid
	} else {
		return PingAssumedIfUnsupported
	}
}


func do_one_connection(c *oneConnection) {
	if !c.Incomming {
		c.SendVersion()
	}

	c.LastDataGot = time.Now()
	c.NextBlocksAsk = time.Now() // askf ro blocks ASAP
	c.NextGetAddr = time.Now().Add(10*time.Second)  // do getaddr ~10 seconds from now
	c.NextPing = time.Now().Add(5*time.Second)  // do first ping ~5 seconds from now

	for !c.Broken {
		c.LoopCnt++
		cmd := c.FetchMessage()
		if c.Broken {
			break
		}

		// Timeout ping in progress
		if c.PingInProgress!=nil && time.Now().After(c.LastPingSent.Add(PingTimeout)) {
			if dbg > 0 {
				println(c.PeerAddr.Ip(), "ping timeout")
			}
			CountSafe("PingTimeout")
			c.HandlePong()  // this will set LastPingSent to nil
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

		CountSafe("rcvd_"+cmd.cmd)
		CountSafeAdd("rbts_"+cmd.cmd, uint64(len(cmd.pl)))
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
				if *server {
					c.SendOwnAddr()
				}

			case "inv":
				c.ProcessInv(cmd.pl)

			case "tx":
				if *txrounting {
					c.ParseTxNet(cmd.pl)
				}

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

			case "pong":
				if c.PingInProgress==nil {
					CountSafe("PongUnexpected")
				} else if bytes.Equal(cmd.pl, c.PingInProgress) {
					c.HandlePong()
				} else {
					CountSafe("PongMismatch")
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
	c.NetConn.Close()
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
		conn.NetConn, e = net.DialTimeout("tcp4", fmt.Sprintf("%d.%d.%d.%d:%d",
			ad.Ip4[0], ad.Ip4[1], ad.Ip4[2], ad.Ip4[3], ad.Port), TCPDialTimeout)
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


// This function should be called only when OutConsActive >= MaxOutCons
func drop_slowest_peer() {
	var worst_ping int
	var worst_conn *oneConnection
	mutex.Lock()
	for _, v := range openCons {
		if v.Incomming && InConsActive < MaxInCons {
			// If this is an incomming connection, but we are not full yet, ignore it
			continue
		}
		ap := v.GetAveragePing()
		if ap > worst_ping {
			worst_ping = ap
			worst_conn = v
		}
	}
	if worst_conn != nil {
		if dbg > 0 {
			println("Droping slowest peer", worst_conn.PeerAddr.Ip(), "/", worst_ping, "ms")
		}
		worst_conn.Broken = true
		CountSafe("PeersDropped")
	}
	mutex.Unlock()
}


func network_process() {
	if *server {
		if *proxy=="" {
			go start_server()
		} else {
			fmt.Println("WARNING: -l switch ignored since -c specified as well")
		}
	}
	next_drop_slowest := time.Now().Add(DropSlowestEvery)
	for {
		mutex.Lock()
		conn_cnt := OutConsActive
		mutex.Unlock()
		if conn_cnt < MaxOutCons {
			adrs := GetBestPeers(16, true)
			if len(adrs)>0 {
				do_network(adrs[mr.Int31n(int32(len(adrs)))])
				continue // do not sleep
			}

			if *proxy=="" && dbg>0 {
				println("no new peers", len(openCons), conn_cnt)
			}
		} else {
			// Having max number of outgoing connections, check to drop the slowest one
			if time.Now().After(next_drop_slowest) {
				drop_slowest_peer()
				next_drop_slowest = time.Now().Add(DropSlowestEvery)
			}
		}
		// If we did not continue, wait a few secs before another loop
		time.Sleep(3e9)
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
				ad, e := NewIncommingPeer(tc.RemoteAddr().String())
				if e == nil {
					conn := NewConnection(ad)
					conn.ConnectedAt = time.Now()
					conn.Incomming = true
					conn.NetConn = tc
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
					println("NewIncommingPeer:", e.Error())
					CountSafe("InConnRefused")
					tc.Close()
				}
			}
		} else {
			time.Sleep(250e6)
		}
	}
}
