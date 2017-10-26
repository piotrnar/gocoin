package network

import (
	"fmt"
	"net"
	"time"
	"sync"
	"bytes"
	"errors"
	"strings"
	"sync/atomic"
	"crypto/rand"
	"encoding/hex"
	"encoding/binary"
	"github.com/piotrnar/gocoin/lib/btc"
	"github.com/piotrnar/gocoin/client/common"
	"github.com/piotrnar/gocoin/lib/others/sys"
	"github.com/piotrnar/gocoin/lib/others/peersdb"
)


const (
	AskAddrsEvery = (5*time.Minute)
	MaxAddrsPerMessage = 500
	SendBufSize = 16*1024*1024 // If you'd have this much in the send buffer, disconnect the peer

	GetHeadersTimeout = 2*time.Minute  // Timeout to receive headers
	VersionMsgTimeout = 20*time.Second  // Timeout to receive teh version message after connecting
	TCPDialTimeout = 10*time.Second // If it does not connect within this time, assume it dead

	MIN_PROTO_VERSION = 209

	HammeringMinReconnect = 60*time.Second // If any incoming peer reconnects in below this time, ban it

	ExpireCachedAfter = 20*time.Minute /*If a block stays in the cache for that long, drop it*/

	MAX_PEERS_BLOCKS_IN_PROGRESS = 500
	MAX_BLOCKS_FORWARD_CNT = 5000 // Never ask for a block higher than current top + this value
	MAX_BLOCKS_FORWARD_SIZ = 50e6
	MAX_GETDATA_FORWARD = 2e6 // Download up to 2MB forward (or one block)

	MAINTANENCE_PERIOD = time.Minute
	NO_INV_TIMEOUT = 15*time.Minute  // If peer sent us no "inv" for that much time, consider it useless

	MAX_INV_HISTORY = 500

	SERVICE_SEGWIT = 0x8

	TxsCounterPeriod = 6*time.Second // how long for one tick
	TxsCounterBufLen = 60 // how many ticks
	OnlineImmunityMinutes = int(TxsCounterBufLen*TxsCounterPeriod/time.Minute)

	PeerTickPeriod = 100*time.Millisecond // run the peer's tick not more often than this
	InvsFlushPeriod = 10*time.Millisecond // send all the pending invs to the peer not more often than this
)


var (
	Mutex_net sync.Mutex
	OpenCons map[uint64]*OneConnection = make(map[uint64]*OneConnection)
	InConsActive, OutConsActive uint32
	LastConnId uint32
	nonce [8]byte

	// Hammering protection (peers that keep re-connecting) map IPv4 => UnixTime
	HammeringMutex sync.Mutex
	RecentlyDisconencted map[[4]byte] time.Time = make(map[[4]byte] time.Time)
)

type NetworkNodeStruct struct {
	Version uint32
	Services uint64
	Timestamp uint64
	Height uint32
	Agent string
	DoNotRelayTxs bool
	ReportedIp4 uint32
	SendHeaders bool

	// BIP152:
	SendCmpctVer uint64
	HighBandwidth bool
}

type ConnectionStatus struct {
	Incomming bool
	ConnectedAt time.Time
	VersionReceived bool
	LastBtsRcvd, LastBtsSent uint32
	LastCmdRcvd, LastCmdSent string
	LastDataGot time.Time // if we have no data for some time, we abort this conenction
	OurGetAddrDone bool // Whether we shoudl issue another "getaddr"

	AllHeadersReceived bool // keep sending getheaders until this is not set
	LastHeadersEmpty bool
	TotalNewHeadersCount int
	GetHeadersInProgress sys.SyncBool
	GetHeadersTimeout time.Time
	LastHeadersHeightAsk uint32
	GetBlocksDataNow sys.SyncBool

	LastSent time.Time
	MaxSentBufSize int

	PingHistory [PingHistoryLength]int
	PingHistoryIdx int
	InvsRecieved uint64

	BytesReceived, BytesSent uint64
	Counters map[string]uint64

	GetAddrDone bool
	MinFeeSPKB int64  // BIP 133

	TxsReceived int // During last hour

	IsSpecial bool // Special connections get more debgs and are not being automatically dropped
}

type ConnInfo struct {
	ID uint32
	PeerIp string

	NetworkNodeStruct
	ConnectionStatus

	BytesToSend int
	BlocksInProgress int
	InvsToSend int
	AveragePing int
	InvsDone int
	BlocksReceived int
	HasImmunity bool

	LocalAddr, RemoteAddr string
}

type OneConnection struct {
	// Source of this IP:
	*peersdb.PeerAddr
	ConnID uint32

	sync.Mutex // protects concurent access to any fields inside this structure

	broken bool // flag that the conenction has been broken / shall be disconnected
	banit bool // Ban this client after disconnecting
	misbehave int // When it reaches 1000, ban it

	net.Conn

	// TCP connection data:
	X ConnectionStatus

	Node NetworkNodeStruct // Data from the version message

	// Messages reception state machine:
	recv struct {
		hdr [24]byte
		hdr_len int
		pl_len uint32 // length taken from the message header
		cmd string
		dat []byte
		datlen uint32
	}
	LastMsgTime time.Time

	InvDone struct {
		Map map[uint64]uint32
		History []uint64
		Idx int
	}

	// Message sending state machine:
	sendBuf [SendBufSize]byte
	SendBufProd, SendBufCons int

	// Statistics:
	PendingInvs []*[36]byte // List of pending INV to send and the mutex protecting access to it

	GetBlockInProgress map[BIDX] *oneBlockDl

	// Ping stats
	LastPingSent time.Time
	PingInProgress []byte

	counters map[string] uint64

	blocksreceived []time.Time
	nextMaintanence time.Time
	nextGetData time.Time

	// we need these three below to count txs received only during last hour
	txsCur int
	txsCha chan int
	txsNxt time.Time
}

type BIDX [btc.Uint256IdxLen]byte

type oneBlockDl struct {
	hash *btc.Uint256
	start time.Time
	col *CmpctBlockCollector
}


type BCmsg struct {
	cmd string
	pl  []byte
}


func NewConnection(ad *peersdb.PeerAddr) (c *OneConnection) {
	c = new(OneConnection)
	c.PeerAddr = ad
	c.GetBlockInProgress = make(map[BIDX] *oneBlockDl)
	c.ConnID = atomic.AddUint32(&LastConnId, 1)
	c.counters = make(map[string]uint64)
	c.InvDone.Map = make(map[uint64]uint32, MAX_INV_HISTORY)
	return
}


func (v *OneConnection) IncCnt(name string, val uint64) {
	v.Mutex.Lock()
	v.counters[name] += val
	v.Mutex.Unlock()
}


func (v *OneConnection) BlksInProgress() (res int) {
	v.Mutex.Lock()
	res = len(v.GetBlockInProgress)
	v.Mutex.Unlock()
	return
}


// call it with locked mutex!
func (v *OneConnection) BytesToSent() int {
	if v.SendBufProd >= v.SendBufCons {
		return v.SendBufProd - v.SendBufCons
	} else {
		return v.SendBufProd + SendBufSize - v.SendBufCons
	}
}


func (v *OneConnection) GetStats(res *ConnInfo) {
	v.Mutex.Lock()
	res.ID = v.ConnID
	res.PeerIp = v.PeerAddr.Ip()
	if v.Conn != nil {
		res.LocalAddr = v.Conn.LocalAddr().String()
		res.RemoteAddr = v.Conn.RemoteAddr().String()
	}
	res.NetworkNodeStruct = v.Node
	res.ConnectionStatus = v.X
	res.BytesToSend = v.BytesToSent()
	res.BlocksInProgress = len(v.GetBlockInProgress)
	res.InvsToSend = len(v.PendingInvs)
	res.AveragePing = v.GetAveragePing()

	res.Counters = make(map[string]uint64, len(v.counters))
	for k, v := range v.counters {
		res.Counters[k] = v
	}

	res.InvsDone = len(v.InvDone.History)
	res.BlocksReceived = len(v.blocksreceived)

	v.Mutex.Unlock()
}


func (c *OneConnection) SendRawMsg(cmd string, pl []byte) (e error) {
	c.Mutex.Lock()
	if !c.broken {
		// we never allow the buffer to be totally full because then producer would be equal consumer
		if bytes_left := SendBufSize - c.BytesToSent(); bytes_left <= len(pl) + 24 {
			c.Mutex.Unlock()
			/*println(c.PeerAddr.Ip(), c.Node.Version, c.Node.Agent, "Peer Send Buffer Overflow @",
				cmd, bytes_left, len(pl)+24, c.SendBufProd, c.SendBufCons, c.BytesToSent())*/
			c.Disconnect("SendBufferOverflow")
			common.CountSafe("PeerSendOverflow")
			return errors.New("Send buffer overflow")
		}

		c.counters["sent_"+cmd]++
		c.counters["sbts_"+cmd] += uint64(len(pl))

		common.CountSafe("sent_"+cmd)
		common.CountSafeAdd("sbts_"+cmd, uint64(len(pl)))
		var sbuf [24]byte

		c.X.LastCmdSent = cmd
		c.X.LastBtsSent = uint32(len(pl))

		binary.LittleEndian.PutUint32(sbuf[0:4], common.Version)
		copy(sbuf[0:4], common.Magic[:])
		copy(sbuf[4:16], cmd)
		binary.LittleEndian.PutUint32(sbuf[16:20], uint32(len(pl)))

		sh := btc.Sha2Sum(pl[:])
		copy(sbuf[20:24], sh[:4])

		c.append_to_send_buffer(sbuf[:])
		c.append_to_send_buffer(pl)

		if x:=c.BytesToSent(); x>c.X.MaxSentBufSize {
			c.X.MaxSentBufSize = x
		}
	}
	c.Mutex.Unlock()
	return
}



// this function assumes that there is enough room inside sendBuf
func (c *OneConnection) append_to_send_buffer(d []byte) {
	room_left := SendBufSize - c.SendBufProd
	if room_left>=len(d) {
		copy(c.sendBuf[c.SendBufProd:], d)
		room_left = c.SendBufProd+len(d)
		if room_left>=SendBufSize {
			c.SendBufProd = 0
		} else {
			c.SendBufProd = room_left
		}
	} else {
		copy(c.sendBuf[c.SendBufProd:], d[:room_left])
		copy(c.sendBuf[:], d[room_left:])
		c.SendBufProd = c.SendBufProd+len(d)-SendBufSize
	}
}


func (c *OneConnection) Disconnect(why string) {
	c.Mutex.Lock()
	if c.X.IsSpecial || common.DebugLevel != 0 {
		print("Disconnect " + c.PeerAddr.Ip() + " (" + c.Node.Agent + ") because " + why + "\n> ")
	}
	c.broken = true
	c.Mutex.Unlock()
}


func (c *OneConnection) IsBroken() (res bool) {
	c.Mutex.Lock()
	res = c.broken
	c.Mutex.Unlock()
	return
}


func (c *OneConnection) DoS(why string) {
	common.CountSafe("Ban"+why)
	c.Mutex.Lock()
	if c.X.IsSpecial || common.DebugLevel != 0 {
		print("BAN " + c.PeerAddr.Ip() + " (" + c.Node.Agent + ") because " + why + "\n> ")
	}
	c.banit = true
	c.broken = true
	c.Mutex.Unlock()
}


func (c *OneConnection) Misbehave(why string, how_much int) (res bool) {
	c.Mutex.Lock()
	if c.X.IsSpecial || common.DebugLevel != 0 {
		print("Misbehave " + c.PeerAddr.Ip() + " (" + c.Node.Agent + ") because " + why + "\n> ")
	}
	if !c.banit {
		common.CountSafe("Bad"+why)
		c.misbehave += how_much
		if c.misbehave >= 1000 {
			common.CountSafe("BanMisbehave")
			res = true
			c.banit = true
			c.broken = true
			//print("Ban " + c.PeerAddr.Ip() + " (" + c.Node.Agent + ") because " + why + "\n> ")
		}
	}
	c.Mutex.Unlock()
	return
}


func (c *OneConnection) HandleError(e error) (error) {
	if nerr, ok := e.(net.Error); ok && nerr.Timeout() {
		//fmt.Println("Just a timeout - ignore")
		return nil
	}
	c.recv.hdr_len = 0
	c.recv.dat = nil
	c.Disconnect("Error:"+e.Error())
	return e
}


func (c *OneConnection) FetchMessage() (*BCmsg) {
	var e error
	var n int

	for c.recv.hdr_len < 24 {
		n, e = common.SockRead(c.Conn, c.recv.hdr[c.recv.hdr_len:24])
		c.Mutex.Lock()
		if n > 0 {
			c.X.LastDataGot = time.Now()
			c.recv.hdr_len += n
		}
		if e != nil {
			c.Mutex.Unlock()
			c.HandleError(e)
			return nil // Make sure to exit here, in case of timeout
		}
		if c.recv.hdr_len >= 4 && !bytes.Equal(c.recv.hdr[:4], common.Magic[:]) {
			c.Mutex.Unlock()
			if common.DebugLevel >0 {
				println("FetchMessage: Proto out of sync")
			}
			fmt.Printf("BadMagic from %s %s \n hdr:%s  n:%d\n> ", c.PeerAddr.Ip(), c.Node.Agent,
				hex.EncodeToString(c.recv.hdr[:c.recv.hdr_len]), n)
			common.CountSafe("NetBadMagic")
			c.Disconnect("BadMagic")
			return nil
		}
		if c.broken {
			c.Mutex.Unlock()
			return nil
		}
		if c.recv.hdr_len >= 24 {
			c.recv.pl_len = binary.LittleEndian.Uint32(c.recv.hdr[16:20])
			c.recv.cmd = strings.TrimRight(string(c.recv.hdr[4:16]), "\000")
		}
		c.Mutex.Unlock()
	}

	if c.recv.pl_len > 0 {
		if c.recv.dat == nil {
			msi := maxmsgsize(c.recv.cmd)
			if c.recv.pl_len > msi {
				c.DoS("Big-"+c.recv.cmd)
				return nil
			}
			c.Mutex.Lock()
			c.recv.dat = make([]byte, c.recv.pl_len)
			c.recv.datlen = 0
			c.Mutex.Unlock()
		}
		for c.recv.datlen < c.recv.pl_len {
			n, e = common.SockRead(c.Conn, c.recv.dat[c.recv.datlen:])
			if n > 0 {
				c.Mutex.Lock()
				c.recv.datlen += uint32(n)
				c.Mutex.Unlock()
				if c.recv.datlen > c.recv.pl_len {
					println(c.PeerAddr.Ip(), "is sending more of", c.recv.cmd, "then it should have", c.recv.datlen, c.recv.pl_len)
					c.DoS("MsgSizeMismatch")
					return nil
				}
			}
			if e != nil {
				c.HandleError(e)
				return nil
			}
			if c.broken {
				return nil
			}
		}
	}

	sh := btc.Sha2Sum(c.recv.dat)
	if !bytes.Equal(c.recv.hdr[20:24], sh[:4]) {
		//println(c.PeerAddr.Ip(), "Msg checksum error")
		c.DoS("MsgBadChksum")
		return nil
	}

	ret := new(BCmsg)
	ret.cmd = c.recv.cmd
	ret.pl = c.recv.dat

	c.Mutex.Lock()
	c.recv.dat = nil
	c.recv.hdr_len = 0
	c.X.BytesReceived += uint64(24+len(ret.pl))
	c.Mutex.Unlock()

	c.LastMsgTime = time.Now()

	return ret
}


func ConnectionActive(ad *peersdb.PeerAddr) (yes bool) {
	Mutex_net.Lock()
	_, yes = OpenCons[ad.UniqID()]
	Mutex_net.Unlock()
	return
}


// Returns maximum accepted payload size of a given type of message
func maxmsgsize(cmd string) uint32 {
	switch cmd {
		case "inv": return 3+50000*36 // the spec says "max 50000 entries"
		case "tx": return 500e3 // max segwit tx size 500KB
		case "addr": return 3+1000*30 // max 1000 addrs
		case "block": return 8e6 // max seg2x block size 8MB
		case "getblocks": return 4+3+500*32+32 // we allow up to 500 locator hashes
		case "getdata": return 3+50000*36 // the spec says "max 50000 entries"
		case "headers": return 3+50000*36 // the spec says "max 50000 entries"
		case "getheaders": return 4+3+500*32+32 // we allow up to 500 locator hashes
		case "cmpctblock": return 0x10000 // 64KB shall be enough
		case "getblocktxn": return 0x10000 // 64KB shall be enough
		case "blocktxn": return 32+2+1e6-80-5 // all txs that can fit withing 1MB block
		default: return 1024 // Any other type of block: 1KB payload limit
	}
}


func NetCloseAll() {
	println("Closing network")
	common.NetworkClosed.Set()
	time.Sleep(1e9) // give one second for WebUI requests to complete
	common.LockCfg()
	common.SetListenTCP(false, false)
	common.UnlockCfg()
	Mutex_net.Lock()
	if InConsActive > 0 || OutConsActive > 0 {
		for _, v := range OpenCons {
			v.Disconnect("CloseAll")
		}
	}
	Mutex_net.Unlock()
	for {
		Mutex_net.Lock()
		all_done := InConsActive == 0 && OutConsActive == 0
		Mutex_net.Unlock()
		if all_done {
			return
		}
		time.Sleep(1e7)
	}
	for TCPServerStarted {
		time.Sleep(1e7) // give one second for all the pending messages to get processed
	}
}


func DropPeer(conid uint32) {
	Mutex_net.Lock()
	defer Mutex_net.Unlock()
	for _, v := range OpenCons {
		if uint32(conid)==v.ConnID {
			v.DoS("FromUI")
			//fmt.Println("The connection with", v.PeerAddr.Ip(), "is being dropped and the peer is banned")
			return
		}
	}
	fmt.Println("DropPeer: There is no such an active connection", conid)
}


func init() {
	rand.Read(nonce[:])
}
