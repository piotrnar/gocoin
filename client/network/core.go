package network

import (
	"bytes"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/piotrnar/gocoin/client/common"
	"github.com/piotrnar/gocoin/client/network/peersdb"
	"github.com/piotrnar/gocoin/lib/btc"
	"github.com/piotrnar/gocoin/lib/others/sys"
)

const (
	AskAddrsEvery      = (5 * time.Minute)
	MaxAddrsPerMessage = 500
	SendBufSize        = 16 * 1024 * 1024 // If you'd have this much in the send buffer, disconnect the peer
	SendBufMask        = SendBufSize - 1

	GetHeadersTimeout = 2 * time.Minute  // Timeout to receive headers
	VersionMsgTimeout = 20 * time.Second // Timeout to receive the version message after connecting
	TCPDialTimeout    = 20 * time.Second // If it does not connect within this time, assume it dead
	NoDataTimeout     = 5 * time.Minute  // If no data received within this time, disconnect

	MIN_PROTO_VERSION = 209

	HammeringMinReconnect    = 20 * time.Minute // If any incoming peer reconnects in below this time, ban it
	HammeringMaxAllowedCount = 5                // If reconnecting more than this many times within the time above, ban the IP
	HammeringExpirePeriod    = time.Minute      // Expite Hammering history interval

	ExpireCachedAfter = 20 * time.Minute /*If a block stays in the cache for that long, drop it*/

	MAX_PEERS_BLOCKS_IN_PROGRESS = 500
	MAX_BLOCKS_FORWARD_CNT       = 5000  // Never ask for a block higher than current top + this value
	MAX_BLOCKS_FORWARD_SIZ       = 500e6 // this  will store about that much blocks data in RAM
	MAX_GETDATA_FORWARD          = 2e6   // Download up to 2MB forward (or one block)

	MAINTANENCE_PERIOD = time.Minute

	MAX_INV_HISTORY = 500

	SERVICE_SEGWIT = 0x8

	TxsCounterPeriod      = 6 * time.Second // how long for one tick
	TxsCounterBufLen      = 60              // how many ticks
	OnlineImmunityMinutes = int(TxsCounterBufLen * TxsCounterPeriod / time.Minute)

	PeerTickPeriod  = 100 * time.Millisecond // run the peer's tick not more often than this
	InvsFlushPeriod = 10 * time.Millisecond  // send all the pending invs to the peer not more often than this

	MAX_GETMP_TXS = 1e6
)

var (
	Mutex_net                   sync.Mutex
	OpenCons                    map[uint64]*OneConnection = make(map[uint64]*OneConnection)
	InConsActive, OutConsActive uint32
	LastConnId                  uint32
	nonce                       [8]byte

	// Hammering protection (peers that keep re-connecting) map IPv4 => UnixTime
	HammeringMutex       sync.Mutex
	RecentlyDisconencted map[[4]byte]*RecentlyDisconenctedType = make(map[[4]byte]*RecentlyDisconenctedType)
)

type RecentlyDisconenctedType struct {
	time.Time
	Count uint
	Why   string
}

type NetworkNodeStruct struct {
	Version       uint32
	Services      uint64
	Timestamp     uint64
	Height        uint32
	Agent         string
	DoNotRelayTxs bool
	ReportedIp4   uint32
	SendHeaders   bool
	Nonce         [8]byte

	// BIP152:
	SendCmpctVer  uint64
	HighBandwidth bool
}

type ConnectionStatus struct {
	Incomming                bool
	ConnectedAt              time.Time
	VersionReceived          bool
	LastBtsRcvd, LastBtsSent uint32
	LastCmdRcvd, LastCmdSent string
	LastDataGot              time.Time // if we have no data for some time, we abort this conenction
	Ticks                    uint64
	OurGetAddrDone           bool // Whether we shoudl issue another "getaddr"

	AllHeadersReceived      bool // keep sending getheaders until this is not set
	LastHeadersEmpty        bool
	TotalNewHeadersCount    int
	GetHeadersInProgress    bool
	GetHeadersTimeOutAt     time.Time
	GetHeadersSentAtPingCnt uint64
	LastHeadersHeightAsk    uint32
	GetBlocksDataNow        bool

	LastSent       time.Time
	MaxSentBufSize int

	PingHistory    [PingHistoryLength]int
	PingHistoryIdx int
	InvsRecieved   uint64

	BytesReceived, BytesSent uint64
	Counters                 map[string]uint64

	GetAddrDone bool
	MinFeeSPKB  int64 // BIP 133

	TxsReceived int // During last hour

	IsSpecial bool // Special connections get more debgs and are not being automatically dropped
	IsGocoin  bool
	Debug     bool

	Authorized bool
	AuthMsgGot uint
	AuthAckGot bool

	LastMinFeePerKByte uint64

	PingSentCnt   uint64
	BlocksExpired uint64

	NewAddrsRcvd uint64
	AddrMsgsRcvd uint64

	ChainSynchronized bool // Initiated by "auth" or "autack" message (gocoin specific commmands)
}

type ConnInfo struct {
	ID     uint32
	PeerIp string

	NetworkNodeStruct
	ConnectionStatus

	BytesToSend      int
	BlocksInProgress int
	InvsToSend       int
	AveragePing      int
	InvsDone         int
	BlocksReceived   int
	GetMPInProgress  bool

	LocalAddr, RemoteAddr string

	// This one is only set inside webui's hnadler (for sorted connections)
	HasImmunity bool
}

type OneConnection struct {
	// Source of this IP:
	*peersdb.PeerAddr
	ConnID uint32

	sync.Mutex // protects concurent access to any fields inside this structure

	broken           bool // flag that the conenction has been broken / shall be disconnected
	dead             bool // If set the alive time in PeersDB will ne moved back
	why_disconnected string
	banit            bool // Ban this client after disconnecting
	ban_reason       string
	misbehave        int // When it reaches 1000, ban it

	net.Conn

	// TCP connection data:
	X ConnectionStatus

	Node NetworkNodeStruct // Data from the version message

	// Messages reception state machine:
	recv struct {
		hdr     [24]byte
		hdr_len int
		pl_len  uint32 // length taken from the message header
		cmd     string
		dat     []byte
		datlen  uint32
	}
	LastMsgTime        time.Time
	unfinished_getdata []byte

	InvDone struct {
		Map     map[uint64]uint32
		History []uint64
		Idx     int
	}

	// Message sending state machine:
	sendBuf                  [SendBufSize]byte
	SendBufProd, SendBufCons int

	// Statistics:
	PendingInvs []*[36]byte // List of pending INV to send and the mutex protecting access to it
	sendInvsNow sys.SyncBool

	GetBlockInProgress map[BIDX]*oneBlockDl

	// Ping stats
	LastPingSent   time.Time
	PingInProgress []byte

	counters map[string]uint64

	blocksreceived  []time.Time
	nextMaintanence time.Time
	nextGetData     time.Time

	// we need these three below to count txs received only during last hour
	txsCur int
	txsCha chan int
	txsNxt time.Time

	writing_thread_done sync.WaitGroup
	writing_thread_push chan bool

	GetMP chan bool
}

type BIDX [btc.Uint256IdxLen]byte

type oneBlockDl struct {
	hash          *btc.Uint256
	start         time.Time
	col           *CmpctBlockCollector
	SentAtPingCnt uint64
}

type BCmsg struct {
	cmd string
	pl  []byte
}

func NewConnection(ad *peersdb.PeerAddr) (c *OneConnection) {
	c = new(OneConnection)
	c.PeerAddr = ad
	c.GetBlockInProgress = make(map[BIDX]*oneBlockDl)
	c.ConnID = atomic.AddUint32(&LastConnId, 1)
	c.counters = make(map[string]uint64)
	c.InvDone.Map = make(map[uint64]uint32, MAX_INV_HISTORY)
	c.GetMP = make(chan bool, 1)
	return
}

func (v *OneConnection) cntLockInc(name string) {
	v.Mutex.Lock()
	v.counters[name]++
	v.Mutex.Unlock()
}

func (v *OneConnection) cntInc(name string) {
	v.counters[name]++
}

func (v *OneConnection) cntAdd(name string, val uint64) {
	v.counters[name] += val
}

// MutexSetBool does a mutex protected assignment of val to addr.
func (v *OneConnection) MutexSetBool(addr *bool, val bool) {
	v.Mutex.Lock()
	*addr = val
	v.Mutex.Unlock()
}

// MutexGetBool does a mutex protected read of the value of addr and returns it.
func (v *OneConnection) MutexGetBool(addr *bool) (val bool) {
	v.Mutex.Lock()
	val = *addr
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

// call it with unlocked mutex!
func (c *OneConnection) SendingPaused() (res bool) {
	c.Mutex.Lock()
	res = c.BytesToSent() > SendBufSize/2
	c.Mutex.Unlock()
	return
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
	res.GetMPInProgress = len(v.GetMP) != 0

	v.Mutex.Unlock()
}

func (c *OneConnection) SendRawMsg(cmd string, pl []byte) (e error) {
	c.Mutex.Lock()

	/*if c.X.Debug {
		fmt.Println(c.ConnID, "sent", cmd, len(pl))
	}*/

	if !c.broken {
		// we never allow the buffer to be totally full because then producer would be equal consumer
		if bytes_left := SendBufSize - c.BytesToSent(); bytes_left <= len(pl)+24 {
			c.Mutex.Unlock()
			println(c.PeerAddr.Ip(), c.Node.Version, c.Node.Agent, "Peer Send Buffer Overflow @",
				cmd, bytes_left, len(pl)+24, c.SendBufProd, c.SendBufCons, c.BytesToSent())
			c.DoS("SendBufferOverflow")
			common.CountSafe("PeerSendOverflow")
			return errors.New("send buffer overflow")
		}

		c.cntInc("sent_" + cmd)
		c.cntAdd("sbts_"+cmd, uint64(len(pl)))

		common.CountSafe("sent_" + cmd)
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

		if x := c.BytesToSent(); x > c.X.MaxSentBufSize {
			c.X.MaxSentBufSize = x
		}
	}
	c.Mutex.Unlock()
	select {
	case c.writing_thread_push <- true:
	default:
	}
	return
}

// append_to_send_buffer assumes that there is enough room inside sendBuf.
func (c *OneConnection) append_to_send_buffer(d []byte) {
	room_left := SendBufSize - c.SendBufProd
	if room_left >= len(d) {
		copy(c.sendBuf[c.SendBufProd:], d)
	} else {
		copy(c.sendBuf[c.SendBufProd:], d[:room_left])
		copy(c.sendBuf[:], d[room_left:])
	}
	c.SendBufProd = (c.SendBufProd + len(d)) & SendBufMask
}

func (c *OneConnection) Disconnect(dead bool, why string) {
	c.Mutex.Lock()
	if c.X.Debug {
		print("Disconnect " + c.PeerAddr.Ip() + " (" + c.Node.Agent + ") because -" + why + "-\n> ")
		//println("LastCmdSent:", c.X.LastCmdSent, c.X.LastBtsSent, "   LastCmdRcvd:", c.X.LastCmdRcvd, c.X.LastBtsRcvd)
	}
	c.dead = dead
	c.broken = true
	c.why_disconnected = why
	c.Mutex.Unlock()
}

func (c *OneConnection) IsBroken() (res bool) {
	c.Mutex.Lock()
	res = c.broken
	c.Mutex.Unlock()
	return
}

func (c *OneConnection) DoS(why string) {
	common.CountSafe("Ban" + why)
	c.Mutex.Lock()
	if c.X.Debug || c.X.Authorized {
		print("BAN " + c.PeerAddr.Ip() + " (" + c.Node.Agent + ") because " + why + "\n> ")
	}
	c.banit = true
	c.ban_reason = why
	c.broken = true
	c.Mutex.Unlock()
}

func (c *OneConnection) Misbehave(why string, how_much int) (res bool) {
	c.Mutex.Lock()
	if c.X.Debug || c.X.Authorized {
		print("Misbehave " + c.PeerAddr.Ip() + " (" + c.Node.Agent + ") because " + why + "\n> ")
	}
	if !c.banit {
		common.CountSafe("Bad" + why)
		c.misbehave += how_much
		if c.misbehave >= 1000 {
			common.CountSafe("BanMisbehave")
			res = true
			c.banit = true
			c.ban_reason = "Bad" + why
			c.broken = true
			//print("Ban " + c.PeerAddr.Ip() + " (" + c.Node.Agent + ") because " + why + "\n> ")
		}
	} else {
		common.CountSafe("Misb" + why)
	}
	c.Mutex.Unlock()
	return
}

func (c *OneConnection) HandleError(e error) error {
	if nerr, ok := e.(net.Error); ok && nerr.Timeout() {
		//fmt.Println("Just a timeout - ignore")
		return nil
	}
	c.recv.hdr_len = 0
	c.recv.dat = nil
	c.Disconnect(true, "Error:"+e.Error())
	return e
}

func (c *OneConnection) FetchMessage() (ret *BCmsg, timeout_or_data bool) {
	var e error
	var n int

	for c.recv.hdr_len < 24 {
		n, e = common.SockRead(c.Conn, c.recv.hdr[c.recv.hdr_len:24])
		if n < 0 {
			n = 0
		} else {
			timeout_or_data = true
		}
		c.Mutex.Lock()
		if n > 0 {
			c.X.BytesReceived += uint64(n)
			c.X.LastDataGot = time.Now()
			c.recv.hdr_len += n
		}
		if e != nil {
			c.Mutex.Unlock()
			c.HandleError(e)
			return // Make sure to exit here, in case of timeout
		}
		if c.recv.hdr_len >= 4 && !bytes.Equal(c.recv.hdr[:4], common.Magic[:]) {
			if c.X.Debug {
				fmt.Printf("BadMagic from %s %s \n hdr:%s  n:%d\n R: %s %d / S: %s %d\n> ", c.PeerAddr.Ip(), c.Node.Agent,
					hex.EncodeToString(c.recv.hdr[:c.recv.hdr_len]), n,
					c.X.LastCmdRcvd, c.X.LastBtsRcvd, c.X.LastCmdSent, c.X.LastBtsSent)
			}
			c.Mutex.Unlock()
			common.CountSafe("NetBadMagic")
			c.Ban("NetBadMagic")
			return
		}
		if c.broken {
			c.Mutex.Unlock()
			return
		}
		if c.recv.hdr_len == 24 {
			c.recv.pl_len = binary.LittleEndian.Uint32(c.recv.hdr[16:20])
			c.recv.cmd = strings.TrimRight(string(c.recv.hdr[4:16]), "\000")
			c.Mutex.Unlock()
		} else {
			if c.recv.hdr_len > 24 {
				// hard to belive but I saw this happening twice on Windows
				println("ERROR: hdr_len > 24 after receiving", n, "bytes")
				c.Mutex.Unlock()
				common.CountSafe("SockReadOverflow")
				c.Disconnect(true, "SockReadOverflow")
				return
			}
			c.Mutex.Unlock()
			return
		}
	}

	if c.recv.pl_len > 0 {
		if c.recv.dat == nil {
			msi := maxmsgsize(c.recv.cmd)
			if c.recv.pl_len > msi {
				c.DoS("Big-" + c.recv.cmd)
				return
			}
			c.Mutex.Lock()
			c.recv.dat = make([]byte, c.recv.pl_len)
			c.recv.datlen = 0
			c.Mutex.Unlock()
		}
		if c.recv.datlen < c.recv.pl_len {
			n, e = common.SockRead(c.Conn, c.recv.dat[c.recv.datlen:])
			if n < 0 {
				n = 0
			} else {
				timeout_or_data = true
			}
			if n > 0 {
				c.Mutex.Lock()
				c.X.BytesReceived += uint64(n)
				c.recv.datlen += uint32(n)
				c.Mutex.Unlock()
				if c.recv.datlen > c.recv.pl_len {
					println(c.PeerAddr.Ip(), "is sending more of", c.recv.cmd, "then it should have", c.recv.datlen, c.recv.pl_len)
					c.DoS("MsgSizeMismatch")
					return
				}
			}
			if e != nil {
				c.HandleError(e)
				return
			}
			if c.MutexGetBool(&c.broken) || c.recv.datlen < c.recv.pl_len {
				return
			}
		}
	}

	sh := btc.Sha2Sum(c.recv.dat)
	if !bytes.Equal(c.recv.hdr[20:24], sh[:4]) {
		//println(c.PeerAddr.Ip(), "Msg checksum error")
		c.DoS("MsgBadChksum")
		return
	}

	ret = new(BCmsg)
	ret.cmd = c.recv.cmd
	ret.pl = c.recv.dat

	c.Mutex.Lock()
	c.recv.hdr_len = 0
	c.recv.cmd = ""
	c.recv.dat = nil

	c.cntInc("rcvd_" + ret.cmd)
	c.cntAdd("rbts_"+ret.cmd, uint64(len(ret.pl)))
	c.X.LastCmdRcvd = ret.cmd
	c.X.LastBtsRcvd = uint32(len(ret.pl))

	c.Mutex.Unlock()

	c.LastMsgTime = time.Now()
	common.CountSafe("rcvd_" + ret.cmd)
	common.CountSafeAdd("rbts_"+ret.cmd, uint64(len(ret.pl)))

	/*if c.X.Debug {
		fmt.Println(c.ConnID, "rcvd", cmd.cmd, len(cmd.pl))
	}*/

	return
}

// Check c.X.AuthAckGot before calling this function
func (c *OneConnection) GetMPNow() {
	if common.GetBool(&common.CFG.TXPool.Enabled) {
		select {
		case c.GetMP <- true:
		default:
			fmt.Println(c.ConnID, "GetMP channel full")
		}
	}
}

func (c *OneConnection) writing_thread() {
	for !c.IsBroken() {
		c.Mutex.Lock() // protect access to c.SendBufProd

		if c.SendBufProd == c.SendBufCons {
			c.Mutex.Unlock()
			// wait for a new write, but time out just in case
			select {
			case <-c.writing_thread_push:
			case <-time.After(10 * time.Millisecond):
			}
			continue
		}

		bytes_to_send := c.SendBufProd - c.SendBufCons
		c.Mutex.Unlock() // unprotect access to c.SendBufProd

		if bytes_to_send < 0 {
			bytes_to_send += SendBufSize
		}
		if c.SendBufCons+bytes_to_send > SendBufSize {
			bytes_to_send = SendBufSize - c.SendBufCons
		}

		n, e := common.SockWrite(c.Conn, c.sendBuf[c.SendBufCons:c.SendBufCons+bytes_to_send])
		if n > 0 {
			c.Mutex.Lock()
			c.X.LastSent = time.Now()
			c.X.BytesSent += uint64(n)
			c.SendBufCons = (c.SendBufCons + n) & SendBufMask
			c.Mutex.Unlock()
		} else if e != nil {
			c.Disconnect(true, "SendErr:"+e.Error())
		} else if n < 0 {
			// It comes here if we could not send a single byte because of BW limit
			time.Sleep(10 * time.Millisecond)
		}
	}
	c.writing_thread_done.Done()
}

func ConnectionActive(ad *peersdb.PeerAddr) (yes bool) {
	Mutex_net.Lock()
	_, yes = OpenCons[ad.UniqID()]
	Mutex_net.Unlock()
	return
}

// maxmsgsize returns maximum accepted payload size of a given type of message.
// For wider compatibility, we assume that any var_len may be up to 9 bytes.
func maxmsgsize(cmd string) uint32 {
	switch cmd {
	case "inv":
		return 9 + 50000*36 // the spec says "max 50000 entries"
	case "tx":
		return 500e3 // max segwit tx size 500KB
	case "addr":
		return 9 + 1000*30 // max 1000 addrs
	case "block":
		return 4e6 // max segwit block size 4MB
	case "getblocks":
		return 4 + 9 + 101*32 + 32 // MAX_LOCATOR_SZ = 101
	case "getdata":
		return 9 + 50000*36 // core: MAX_INV_SZ = 50000
	case "headers":
		return 9 + 2000*89 // core: MAX_HEADERS_RESULTS = 2000
	case "getheaders":
		return 4 + 9 + 101*32 + 32 // MAX_LOCATOR_SZ = 101
	case "cmpctblock":
		return 1e6 // 1MB shall be enough
	case "getblocktxn":
		return 1e6 // 1MB shall be enough
	case "blocktxn":
		return 4e6 // all txs that can fit withing max size block
	case "notfound":
		return 9 + 50000*36 // same as maximum size of getdata
	case "getmp":
		return 9 + 8*MAX_GETMP_TXS
	default:
		return 1024 // Any other type of block: maximum 1KB payload limit
	}
}

func NetCloseAll() {
	sta := time.Now()
	println("Closing network")
	common.NetworkClosed.Set()
	common.SetBool(&common.ListenTCP, false)
	Mutex_net.Lock()
	if InConsActive > 0 || OutConsActive > 0 {
		for _, v := range OpenCons {
			v.Disconnect(false, "CloseAll")
		}
	}
	Mutex_net.Unlock()
	time.Sleep(1e9) // give one second for WebUI requests to complete
	// now wait for all the connections to close
	for {
		Mutex_net.Lock()
		all_done := len(OpenCons) == 0
		Mutex_net.Unlock()
		if all_done {
			return
		}
		if time.Since(sta) > 5*time.Second {
			Mutex_net.Lock()
			fmt.Println("Still have open connections:", InConsActive, OutConsActive, len(OpenCons), "- please report")
			Mutex_net.Unlock()
			break
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
		if uint32(conid) == v.ConnID {
			if v.X.IsSpecial {
				v.Disconnect(false, "FromUI")
			} else {
				v.DoS("FromUI")
			}
			return
		}
	}
	fmt.Println("DropPeer: There is no such an active connection", conid)
}

// GetMP() is called from UI, to force asking the given peer for its mempool
func GetMP(conid uint32) {
	Mutex_net.Lock()
	for _, v := range OpenCons {
		if uint32(conid) == v.ConnID {
			Mutex_net.Unlock()
			v.Mutex.Lock()
			yes := v.X.AuthAckGot // don't bother if we are not authorized there
			v.Mutex.Unlock()
			if yes {
				v.GetMPNow()
			}
			return
		}
	}
	Mutex_net.Unlock()
	fmt.Println("GetMP: There is no such an active connection", conid)
}

func GetMoreHeaders() {
	Mutex_net.Lock()
	for _, v := range OpenCons {
		v.ReceiveHeadersNow()
	}
	Mutex_net.Unlock()
}

func BlocksToGetCnt() (res int) {
	MutexRcv.Lock()
	res = len(BlocksToGet)
	MutexRcv.Unlock()
	return
}

func init() {
	rand.Read(nonce[:])
}
