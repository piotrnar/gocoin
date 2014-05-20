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
	"encoding/binary"
	"github.com/piotrnar/gocoin/btc"
	"github.com/piotrnar/gocoin/chain"
	"github.com/piotrnar/gocoin/client/common"
)


const (
	AskAddrsEvery = (5*time.Minute)
	MaxAddrsPerMessage = 500

	NoDataTimeout = 2*time.Minute
	MaxSendBufferSize = 16*1024*1024 // If you have more than this in the send buffer, disconnect
	SendBufSizeHoldOn = 1000*1000 // If you have more than this in the send buffer do not process any commands

	NewBlocksAskDuration = 5*time.Minute  // Ask each connection for new blocks every X minutes

	GetBlockTimeout = 15*time.Second  // Timeout to receive the entire block (we like it fast)

	TCPDialTimeout = 10*time.Second // If it does not connect within this time, assume it dead
	AnySendTimeout = 30*time.Second // If it does not send a byte within this time, assume it dead

	PingPeriod = 60*time.Second
	PingTimeout = 5*time.Second
	PingHistoryLength = 8
	PingHistoryValid = (PingHistoryLength-4) // Ignore N longest pings
	PingAssumedIfUnsupported = 999 // ms

	DropSlowestEvery = 10*time.Minute // Look for the slowest peer and drop it

	MIN_PROTO_VERSION = 209

	HammeringMinReconnect = 60*time.Second // If any incoming peer reconnects in below this time, ban it
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


type OneConnection struct {
	// Source of this IP:
	PeerAddr *onePeer
	ConnID uint32

	sync.Mutex // protects concurent access to any fields inside this structure

	broken bool // flag that the conenction has been broken / shall be disconnected
	banit bool // Ban this client after disconnecting

	// TCP connection data:
	Incoming bool
	NetConn net.Conn

	// Handshake data
	ConnectedAt time.Time
	VerackReceived bool

	// Data from the version message
	Node struct {
		Version uint32
		Services uint64
		Timestamp uint64
		Height uint32
		Agent string
		DoNotRelayTxs bool
		ReportedIp4 uint32
	}

	// Messages reception state machine:
	recv struct {
		hdr [24]byte
		hdr_len int
		pl_len uint32 // length taken from the message header
		cmd string
		dat []byte
		datlen uint32
	}

	// Message sending state machine:
	Send struct {
		Buf []byte
		LastSent time.Time
	}

	// Statistics:
	LoopCnt, TicksCnt uint  // just to see if the threads loop is alive
	BytesReceived, BytesSent uint64
	LastBtsRcvd, LastBtsSent uint32
	LastCmdRcvd, LastCmdSent string
	InvsRecieved uint64

	PendingInvs []*[36]byte // List of pending INV to send and the mutex protecting access to it

	NextGetAddr time.Time // When we shoudl issue "getaddr" again

	LastDataGot time.Time // if we have no data for some time, we abort this conenction

	LastBlocksFrom *chain.BlockTreeNode // what the last getblocks was based un
	NextBlocksAsk time.Time           // when the next getblocks should be needed

	GetBlockInProgress map[[btc.Uint256IdxLen]byte] *oneBlockDl

	// Ping stats
	PingHistory [PingHistoryLength]int
	PingHistoryIdx int
	NextPing time.Time
	PingInProgress []byte
	LastPingSent time.Time
}

type oneBlockDl struct {
	hash *btc.Uint256
	start time.Time
	head bool
}


type BCmsg struct {
	cmd string
	pl  []byte
}


func NewConnection(ad *onePeer) (c *OneConnection) {
	c = new(OneConnection)
	c.PeerAddr = ad
	c.GetBlockInProgress = make(map[[btc.Uint256IdxLen]byte] *oneBlockDl)
	c.ConnID = atomic.AddUint32(&LastConnId, 1)
	return
}


func (c *OneConnection) SendRawMsg(cmd string, pl []byte) (e error) {
	c.Mutex.Lock()

	if c.Send.Buf!=nil {
		// Before adding more data to the buffer, check the limit
		if len(c.Send.Buf)>MaxSendBufferSize {
			c.Mutex.Unlock()
			if common.DebugLevel > 0 {
				println(c.PeerAddr.Ip(), "Peer Send Buffer Overflow")
			}
			c.Disconnect()
			common.CountSafe("PeerSendOverflow")
			return errors.New("Send buffer overflow")
		}
	} else {
		c.Send.LastSent = time.Now()
	}

	common.CountSafe("sent_"+cmd)
	common.CountSafeAdd("sbts_"+cmd, uint64(len(pl)))
	sbuf := make([]byte, 24+len(pl))

	c.LastCmdSent = cmd
	c.LastBtsSent = uint32(len(pl))

	binary.LittleEndian.PutUint32(sbuf[0:4], common.Version)
	copy(sbuf[0:4], common.Magic[:])
	copy(sbuf[4:16], cmd)
	binary.LittleEndian.PutUint32(sbuf[16:20], uint32(len(pl)))

	sh := btc.Sha2Sum(pl[:])
	copy(sbuf[20:24], sh[:4])
	copy(sbuf[24:], pl)

	c.Send.Buf = append(c.Send.Buf, sbuf...)

	if common.DebugLevel<0 {
		fmt.Println(cmd, len(c.Send.Buf), "->", c.PeerAddr.Ip())
	}
	c.Mutex.Unlock()
	//println(len(c.Send.Buf), "queued for seding to", c.PeerAddr.Ip())
	return
}


func (c *OneConnection) Disconnect() {
	c.Mutex.Lock()
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
	c.banit = true
	c.broken = true
	c.Mutex.Unlock()
}


func (c *OneConnection) HandleError(e error) (error) {
	if nerr, ok := e.(net.Error); ok && nerr.Timeout() {
		//fmt.Println("Just a timeout - ignore")
		return nil
	}
	if common.DebugLevel>0 {
		println("HandleError:", e.Error())
	}
	c.recv.hdr_len = 0
	c.recv.dat = nil
	c.Disconnect()
	return e
}


func (c *OneConnection) FetchMessage() (*BCmsg) {
	var e error
	var n int

	for c.recv.hdr_len < 24 {
		n, e = common.SockRead(c.NetConn, c.recv.hdr[c.recv.hdr_len:24])
		c.Mutex.Lock()
		c.recv.hdr_len += n
		if e != nil {
			c.Mutex.Unlock()
			c.HandleError(e)
			return nil
		}
		if c.recv.hdr_len>=4 && !bytes.Equal(c.recv.hdr[:4], common.Magic[:]) {
			c.Mutex.Unlock()
			if common.DebugLevel >0 {
				println("FetchMessage: Proto out of sync")
			}
			common.CountSafe("NetBadMagic")
			c.Disconnect()
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
				//println(c.PeerAddr.Ip(), "Command", c.recv.cmd, "is going to be too big", c.recv.pl_len, msi)
				c.DoS("MsgTooBig")
				return nil
			}
			c.Mutex.Lock()
			c.recv.dat = make([]byte, c.recv.pl_len)
			c.recv.datlen = 0
			c.Mutex.Unlock()
		}
		for c.recv.datlen < c.recv.pl_len {
			n, e = common.SockRead(c.NetConn, c.recv.dat[c.recv.datlen:])
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
	c.BytesReceived += uint64(24+len(ret.pl))
	c.Mutex.Unlock()

	return ret
}


func ConnectionActive(ad *onePeer) (yes bool) {
	Mutex_net.Lock()
	_, yes = OpenCons[ad.UniqID()]
	Mutex_net.Unlock()
	return
}


// Returns maximum accepted payload size of a given type of message
func maxmsgsize(cmd string) uint32 {
	switch cmd {
		case "inv": return 3+1000*36 // the spec says "max 50000 entries", but we reject more than 1000
		case "tx": return 100e3 // max tx size 100KB
		case "addr": return 3+1000*30 // max 1000 addrs
		case "block": return 1e6 // max block size 1MB
		case "getblocks": return 4+3+500*32+32 // we allow up to 500 locator hashes
		case "getdata": return 3+1000*36 // the spec says "max 50000 entries", but we reject more than 1000
		default: return 1024 // Any other type of block: 1KB payload limit
	}
}


func NetCloseAll() {
	println("Closing network")
	common.NetworkClosed = true
	common.LockCfg()
	common.CFG.Net.ListenTCP = false
	common.UnlockCfg()
	Mutex_net.Lock()
	if InConsActive > 0 || OutConsActive > 0 {
		for _, v := range OpenCons {
			v.Disconnect()
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
}


func init() {
	rand.Read(nonce[:])
}
