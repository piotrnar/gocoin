package main

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
)


const (
	AskAddrsEvery = (5*time.Minute)
	MaxAddrsPerMessage = 500

	NoDataTimeout = 2*time.Minute
	MaxSendBufferSize = 16*1024*1024 // If you have more than this in the send buffer, disconnect
	SendBufSizeHoldOn = 1000*1000 // If you have more than this in the send buffer do not process any commands

	NewBlocksAskDuration = 5*time.Minute  // Ask each connection for new blocks every X minutes

	GetBlockTimeout = 3*time.Minute  // Timeout to receive the entire block
	GetBlockSwitchOffSingle = 15*time.Second // Switch off single mode this time after receiving single block inv

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
	openCons map[uint64]*oneConnection = make(map[uint64]*oneConnection)
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
		pl_len uint32 // length taken from the message header
		cmd string
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


func NewConnection(ad *onePeer) (c *oneConnection) {
	c = new(oneConnection)
	c.PeerAddr = ad
	c.GetBlockInProgress = make(map[[btc.Uint256IdxLen]byte] *oneBlockDl)
	c.ConnID = atomic.AddUint32(&LastConnId, 1)
	return
}


func (c *oneConnection) SendRawMsg(cmd string, pl []byte) (e error) {
	if c.send.buf!=nil {
		if c.send.sofar > MaxSendBufferSize/2 {
			// Try to defragment the buffer
			newbuf := make([]byte, len(c.send.buf)-c.send.sofar)
			copy(newbuf, c.send.buf[c.send.sofar:])
			c.send.buf = newbuf
		}
		// Before adding more data to the buffer, check the limit
		if len(c.send.buf)>MaxSendBufferSize {
			if dbg > 0 {
				println(c.PeerAddr.Ip(), "Peer Send Buffer Overflow")
			}
			c.Broken = true
			CountSafe("PeerSendOverflow")
			return errors.New("Send buffer overflow")
		}
	} else {
		c.send.lastSent = time.Now()
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

	if dbg<0 {
		fmt.Println(cmd, len(c.send.buf), "->", c.PeerAddr.Ip())
	}
	//println(len(c.send.buf), "queued for seding to", c.PeerAddr.Ip())
	return
}


func (c *oneConnection) SendBufferSize() (int) {
	if c.send.buf==nil {
		return 0
	}
	return len(c.send.buf)-c.send.sofar
}


func (c *oneConnection) DoS() {
	CountSafe("BannedNodes")
	c.BanIt = true
	c.Broken = true
	c.recv.hdr_len = 0
	c.recv.pl_len = 0
	c.recv.dat = nil
	c.recv.datlen = 0
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
		c.recv.pl_len = binary.LittleEndian.Uint32(c.recv.hdr[16:20])
		c.recv.cmd = strings.TrimRight(string(c.recv.hdr[4:16]), "\000")
	}

	if c.recv.pl_len > 0 {
		if c.recv.dat == nil {
			msi := maxmsgsize(c.recv.cmd)
			if c.recv.pl_len > msi {
				println(c.PeerAddr.Ip(), "Command", c.recv.cmd, "is going to be too big", c.recv.pl_len, msi)
				c.DoS()
				CountSafe("NetMsgSizeTooBig")
				return nil
			}
			c.recv.dat = make([]byte, c.recv.pl_len)
			c.recv.datlen = 0
		}
		for c.recv.datlen < c.recv.pl_len {
			n, e = SockRead(c.NetConn, c.recv.dat[c.recv.datlen:])
			if n > 0 {
				c.recv.datlen += uint32(n)
				if c.recv.datlen > c.recv.pl_len {
					println(c.PeerAddr.Ip(), "is sending more of", c.recv.cmd, "then it should have",
						c.recv.datlen, c.recv.pl_len)
					c.DoS()
					CountSafe("NetMsgSizeIncorrect")
					return nil
				}
			}
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
		println(c.PeerAddr.Ip(), "Msg checksum error")
		CountSafe("NetBadChksum")
		c.DoS()
		return nil
	}

	ret := new(BCmsg)
	ret.cmd = c.recv.cmd
	ret.pl = c.recv.dat
	c.recv.dat = nil
	c.recv.hdr_len = 0

	c.BytesReceived += uint64(24+len(ret.pl))

	return ret
}


func connectionActive(ad *onePeer) (yes bool) {
	mutex.Lock()
	_, yes = openCons[ad.UniqID()]
	mutex.Unlock()
	return
}


// Returns maximum accepted payload size of a given type of message
func maxmsgsize(cmd string) uint32 {
	switch cmd {
		case "inv": return 2+1000*36 // the spec says "max 50000 entries", but we reject more than 1000
		case "tx": return 100e3 // max tx size 100KB
		case "addr": return 2+1000*30 // max 1000 addrs
		case "block": return 1e6 // max block size 1MB
		case "getblocks": return 4+2+500*32+32 // we allow up to 500 locator hashes
		case "getdata": return 2+1000*36 // the spec says "max 50000 entries", but we reject more than 1000
		default: return 1024 // Any other type of block: 1KB payload limit
	}
}


func init() {
	rand.Read(nonce[:])
}
