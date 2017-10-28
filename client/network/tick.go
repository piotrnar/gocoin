package network

import (
	"os"
	"fmt"
	"net"
	"time"
	"bytes"
	"bufio"
	"strings"
	"math/rand"
	"sync/atomic"
	"encoding/hex"
	"encoding/binary"
	"github.com/piotrnar/gocoin/client/common"
	"github.com/piotrnar/gocoin/lib/others/peersdb"
)


func (c *OneConnection) SendPendingData() bool {
	if c.SendBufProd!=c.SendBufCons {
		bytes_to_send := c.SendBufProd-c.SendBufCons
		if bytes_to_send<0 {
			bytes_to_send += SendBufSize
		}
		if c.SendBufCons+bytes_to_send > SendBufSize {
			bytes_to_send = SendBufSize-c.SendBufCons
		}

		n, e := common.SockWrite(c.Conn, c.sendBuf[c.SendBufCons:c.SendBufCons+bytes_to_send])
		if n > 0 {
			c.Mutex.Lock()
			c.X.LastSent = time.Now()
			c.X.BytesSent += uint64(n)
			n += c.SendBufCons
			if n >= SendBufSize {
				c.SendBufCons = 0
			} else {
				c.SendBufCons = n
			}
			c.Mutex.Unlock()
		} else if e != nil {
			if common.DebugLevel > 0 {
				println(c.PeerAddr.Ip(), "Connection Broken during send")
			}
			c.Disconnect("SendErr:"+e.Error())
		}
	}
	return c.SendBufProd!=c.SendBufCons
}


// Call this once a minute
func (c *OneConnection) Maintanence(now time.Time) {
	// Disconnect and ban useless peers (such that don't send invs)
	if c.X.InvsRecieved==0 && c.X.ConnectedAt.Add(NO_INV_TIMEOUT).Before(now) {
		c.DoS("PeerUseless")
		return
	}

	c.Mutex.Lock()
	defer c.Mutex.Unlock()

	// Expire GetBlockInProgress after five minutes, if they are not in BlocksToGet
	MutexRcv.Lock()
	for k, v := range c.GetBlockInProgress {
		if now.After(v.start.Add(5*time.Minute)) {
			delete(c.GetBlockInProgress, k)
			common.CountSafe("BlockInprogTimeout")
			c.counters["BlockTimeout"]++
			//println(c.ConnID, "GetBlockInProgress timeout")
			if bip, ok := BlocksToGet[k]; ok {
				bip.InProgress--
				continue
			}
			break
		}
	}
	MutexRcv.Unlock()

	// Expire BlocksReceived after two days
	if len(c.blocksreceived)>0 {
		var i int
		for i=0; i<len(c.blocksreceived); i++ {
			if c.blocksreceived[i].Add(common.BlockExpireEvery).After(now) {
				break
			}
			common.CountSafe("BlksRcvdExpired")
		}
		if i>0 {
			//println(c.ConnID, "expire", i, "block(s)")
			c.blocksreceived = c.blocksreceived[i:]
		}
	}
}


func (c *OneConnection) Tick(now time.Time) {
	if !c.X.VersionReceived {
		// Wait only certain amount of time for the version message
		if c.X.ConnectedAt.Add(VersionMsgTimeout).Before(now) {
			c.Disconnect("VersionTimeout")
			common.CountSafe("NetVersionTout")
			if common.DebugLevel > 0 {
				println(c.PeerAddr.Ip(), "version message timeout")
			}
			return
		}
		// If we have no ack, do nothing more.
		return
	}

	if c.X.GetHeadersInProgress.Get() && now.After(c.X.GetHeadersTimeout) {
		//println(c.ConnID, "- GetHdrs Timeout")
		c.Disconnect("HeadersTimeout")
		common.CountSafe("NetHeadersTout")
		return
	}

	// Tick the recent transactions counter
	if now.After(c.txsNxt) {
		c.Mutex.Lock()
		if len(c.txsCha)==cap(c.txsCha) {
			tmp := <- c.txsCha
			c.X.TxsReceived -= tmp
		}
		c.txsCha <- c.txsCur
		c.txsCur = 0
		c.txsNxt = c.txsNxt.Add(TxsCounterPeriod)
		c.Mutex.Unlock()
	}

	if now.After(c.nextMaintanence) {
		c.Maintanence(now)
		c.nextMaintanence = now.Add(MAINTANENCE_PERIOD)
	}

	// Ask node for new addresses...?
	if !c.X.OurGetAddrDone && peersdb.PeerDB.Count() < common.MaxPeersNeeded {
		common.CountSafe("AddrWanted")
		c.SendRawMsg("getaddr", nil)
		c.X.OurGetAddrDone = true
	}

	c.Mutex.Lock()
	ahr := c.X.AllHeadersReceived
	c.Mutex.Unlock()

	if !c.X.GetHeadersInProgress.Get() && !ahr && c.BlksInProgress()==0 {
		c.sendGetHeaders()
	}

	if ahr {
		if !c.X.GetBlocksDataNow.Get() && now.After(c.nextGetData) {
			c.X.GetBlocksDataNow.Set()
		}
		if c.X.GetBlocksDataNow.Get() {
			c.X.GetBlocksDataNow.Clr()
			c.GetBlockData()
		}
	}

	if !c.X.GetHeadersInProgress.Get() && c.BlksInProgress()==0 {
		// Ping if we dont do anything
		c.TryPing()
	}
}

func DoNetwork(ad *peersdb.PeerAddr) {
	var e error
	conn := NewConnection(ad)
	Mutex_net.Lock()
	if _, ok := OpenCons[ad.UniqID()]; ok {
		if common.DebugLevel>0 {
			fmt.Println(ad.Ip(), "already connected")
		}
		common.CountSafe("ConnectingAgain")
		Mutex_net.Unlock()
		return
	}
	if ad.Friend || ad.Manual {
		conn.X.IsSpecial.Set()
	}
	OpenCons[ad.UniqID()] = conn
	OutConsActive++
	Mutex_net.Unlock()
	go func() {
		conn.Conn, e = net.DialTimeout("tcp4", fmt.Sprintf("%d.%d.%d.%d:%d",
			ad.Ip4[0], ad.Ip4[1], ad.Ip4[2], ad.Ip4[3], ad.Port), TCPDialTimeout)
		if e == nil {
			conn.X.ConnectedAt = time.Now()
			if common.DebugLevel!=0 {
				println("Connected to", ad.Ip())
			}
			conn.Run()
		} else {
			if common.DebugLevel!=0 {
				println("Could not connect to", ad.Ip())
			}
			//println(e.Error())
		}
		Mutex_net.Lock()
		delete(OpenCons, ad.UniqID())
		OutConsActive--
		Mutex_net.Unlock()
		ad.Dead()
	}()
}


var (
	TCPServerStarted bool
	next_drop_peer time.Time
	next_clean_hammers time.Time
)


// TCP server
func tcp_server() {
	ad, e := net.ResolveTCPAddr("tcp4", fmt.Sprint("0.0.0.0:", common.DefaultTcpPort))
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

	//fmt.Println("TCP server started at", ad.String())

	for common.IsListenTCP() {
		common.CountSafe("NetServerLoops")
		Mutex_net.Lock()
		ica := InConsActive
		Mutex_net.Unlock()
		if ica < atomic.LoadUint32(&common.CFG.Net.MaxInCons) {
			lis.SetDeadline(time.Now().Add(time.Second))
			tc, e := lis.AcceptTCP()
			if e == nil {
				var terminate bool

				if common.DebugLevel>0 {
					fmt.Println("Incoming connection from", tc.RemoteAddr().String())
				}
				// set port to default, for incomming connections
				ad, e := peersdb.NewPeerFromString(tc.RemoteAddr().String(), true)
				if e == nil {
					// Hammering protection
					HammeringMutex.Lock()
					ti, ok := RecentlyDisconencted[ad.NetAddr.Ip4]
					HammeringMutex.Unlock()
					if ok && time.Now().Sub(ti) < HammeringMinReconnect {
						//println(ad.Ip(), "is hammering within", time.Now().Sub(ti).String())
						common.CountSafe("BanHammerIn")
						ad.Ban()
						terminate = true
					}

					if !terminate {
						// Incoming IP passed all the initial checks - talk to it
						conn := NewConnection(ad)
						conn.X.ConnectedAt = time.Now()
						conn.X.Incomming = true
						conn.Conn = tc
						Mutex_net.Lock()
						if _, ok := OpenCons[ad.UniqID()]; ok {
							//fmt.Println(ad.Ip(), "already connected")
							common.CountSafe("SameIpReconnect")
							Mutex_net.Unlock()
							terminate = true
						} else {
							OpenCons[ad.UniqID()] = conn
							InConsActive++
							Mutex_net.Unlock()
							go func () {
								conn.Run()
								Mutex_net.Lock()
								delete(OpenCons, ad.UniqID())
								InConsActive--
								Mutex_net.Unlock()
							}()
						}
					}
				} else {
					if common.DebugLevel>0 {
						println("NewPeerFromString:", e.Error())
					}
					common.CountSafe("InConnRefused")
					terminate = true
				}

				// had any error occured - close teh TCP connection
				if terminate {
					tc.Close()
				}
			}
		} else {
			time.Sleep(1e9)
		}
	}
	Mutex_net.Lock()
	for _, c := range OpenCons {
		if c.X.Incomming {
			c.Disconnect("CloseAllIn")
		}
	}
	TCPServerStarted = false
	Mutex_net.Unlock()
	//fmt.Println("TCP server stopped")
}


var nextConnectFriends time.Time = time.Now()

func ConnectFriends() {
	f, _ := os.Open("friends.txt")
	if f == nil {
		return
	}
	defer f.Close()
	rd := bufio.NewReader(f)
	if rd != nil {
		for {
			ln, _, er := rd.ReadLine()
			if er != nil {
				break
			}
			ls := strings.SplitN(string(ln), " ", 2)
			ad, _ := peersdb.NewAddrFromString(ls[0], false)
			if ad != nil {
				Mutex_net.Lock()
				curr, _ := OpenCons[ad.UniqID()]
				Mutex_net.Unlock()
				if curr == nil {
					//print("Connecting friend ", ad.Ip(), " ...\n> ")
					ad.Friend = true
					DoNetwork(ad)
				} else {
					curr.PeerAddr.Friend = true
					curr.X.IsSpecial.Set()
				}
			}
		}
	}
}



var TickStage int // TODO: This is to investigate very rare hanging inside NetworkTick()

func NetworkTick() {
	TickStage = 1
	if common.IsListenTCP() {
		if !TCPServerStarted {
			TCPServerStarted = true
			go tcp_server()
		}
	}

	// Push GetHeaders if not in progress
	TickStage = 2
	Mutex_net.Lock()
	TickStage = 3
	var cnt_headers_in_progress int
	var max_headers_got_cnt int
	var _v *OneConnection
	for _, v := range OpenCons {
		TickStage = 31
		v.Mutex.Lock() // TODO: Sometimes it might hang here - check why!!
		TickStage = 32
		if !v.X.AllHeadersReceived || v.X.GetHeadersInProgress.Get() {
			cnt_headers_in_progress++
		} else if !v.X.LastHeadersEmpty {
			if _v==nil || v.X.TotalNewHeadersCount > max_headers_got_cnt {
				max_headers_got_cnt = v.X.TotalNewHeadersCount
				_v = v
			}
		}
		v.Mutex.Unlock()
	}
	TickStage = 4
	conn_cnt := OutConsActive
	Mutex_net.Unlock()

	if cnt_headers_in_progress==0 {
	TickStage = 5
		if _v!=nil {
			common.CountSafe("GetHeadersPush")
			/*println("No headers_in_progress, so take it from", _v.ConnID,
				_v.X.TotalNewHeadersCount, _v.X.LastHeadersEmpty)*/
	TickStage = 51
			_v.Mutex.Lock()
	TickStage = 52
			_v.X.AllHeadersReceived = false
			_v.Mutex.Unlock()
		} else {
	TickStage = 55
			common.CountSafe("GetHeadersNone")
		}
	}

	TickStage = 6
	if common.CFG.DropPeers.DropEachMinutes!=0 {
	TickStage = 7
		if next_drop_peer.IsZero() {
			next_drop_peer = time.Now().Add(common.DropSlowestEvery)
		} else if time.Now().After(next_drop_peer) {
			if drop_worst_peer() {
				next_drop_peer = time.Now().Add(common.DropSlowestEvery)
			} else {
				// If no peer dropped this time, try again sooner
				next_drop_peer = time.Now().Add(common.DropSlowestEvery >> 2)
			}
		}
	}

	TickStage = 8
	// hammering protection - expire recently disconnected
	if next_clean_hammers.IsZero() {
		next_clean_hammers = time.Now().Add(HammeringMinReconnect)
	} else if time.Now().After(next_clean_hammers) {
	TickStage = 9
		HammeringMutex.Lock()
	TickStage = 91
		for k, t := range RecentlyDisconencted {
			if time.Now().Sub(t) >= HammeringMinReconnect {
				delete(RecentlyDisconencted, k)
			}
		}
		HammeringMutex.Unlock()
		next_clean_hammers = time.Now().Add(HammeringMinReconnect)
	}

	// Connect friends
	if time.Now().After(nextConnectFriends) {
		TickStage = 95
		ConnectFriends()
		nextConnectFriends = time.Now().Add(5*time.Minute)
	}

	TickStage = 10
	for conn_cnt < atomic.LoadUint32(&common.CFG.Net.MaxOutCons) {
		var segwit_conns uint32
		if common.CFG.Net.MinSegwitCons > 0 {
			TickStage = 11
			Mutex_net.Lock()
			for _, cc := range OpenCons {
				cc.Mutex.Lock()
				if (cc.Node.Services & SERVICE_SEGWIT) != 0 {
					segwit_conns++
				}
				cc.Mutex.Unlock()
			}
			Mutex_net.Unlock()
		}
		TickStage = 12

		adrs := peersdb.GetBestPeers(128, func(ad *peersdb.PeerAddr) (bool) {
			if segwit_conns<common.CFG.Net.MinSegwitCons && (ad.Services & SERVICE_SEGWIT)==0 {
				return true
			}
			return ConnectionActive(ad)
		})
		if len(adrs)==0 && segwit_conns < common.CFG.Net.MinSegwitCons {
			// we have only non-segwit peers in the database - take them
			adrs = peersdb.GetBestPeers(128, func(ad *peersdb.PeerAddr) (bool) {
				return ConnectionActive(ad)
			})
		}
		if len(adrs)==0 {
		TickStage = 121
			common.LockCfg()
		TickStage = 122
			if common.CFG.ConnectOnly=="" && common.DebugLevel>0 {
				println("no new peers", len(OpenCons), conn_cnt)
			}
			common.UnlockCfg()
			break
		}
		DoNetwork(adrs[rand.Int31n(int32(len(adrs)))])
		TickStage = 13
		Mutex_net.Lock()
		TickStage = 14
		conn_cnt = OutConsActive
		Mutex_net.Unlock()
	}
	TickStage = 0
}


// Process that handles communication with a single peer
func (c *OneConnection) Run() {
	c.SendVersion()

	c.Mutex.Lock()
	now := time.Now()
	c.X.LastDataGot = now
	c.nextMaintanence = now.Add(time.Minute)
	c.LastPingSent = now.Add(5*time.Second-common.PingPeerEvery) // do first ping ~5 seconds from now

	c.txsNxt = now.Add(TxsCounterPeriod)
	c.txsCha = make(chan int, TxsCounterBufLen)

	c.Mutex.Unlock()

	next_tick := now
	next_invs := now

	for !c.IsBroken() {
		if c.IsBroken() {
			break
		}

		if c.SendPendingData() {
			continue // Do now read the socket if we have pending data to send
		}

		now = time.Now()

		if c.X.VersionReceived && now.After(next_invs) {
			c.SendInvs()
			next_invs = now.Add(InvsFlushPeriod)
		}

		if now.After(next_tick) {
			c.Tick(now)
			next_tick = now.Add(PeerTickPeriod)
		}

		cmd := c.FetchMessage()
		if cmd == nil {
			continue
		}

		if c.X.VersionReceived {
			c.PeerAddr.Alive()
		}

		c.Mutex.Lock()
		c.counters["rcvd_"+cmd.cmd]++
		c.counters["rbts_"+cmd.cmd] += uint64(len(cmd.pl))
		c.X.LastCmdRcvd = cmd.cmd
		c.X.LastBtsRcvd = uint32(len(cmd.pl))
		c.Mutex.Unlock()

		if common.DebugLevel<0 {
			fmt.Println(c.PeerAddr.Ip(), "->", cmd.cmd, len(cmd.pl))
		}

		common.CountSafe("rcvd_"+cmd.cmd)
		common.CountSafeAdd("rbts_"+cmd.cmd, uint64(len(cmd.pl)))

		if cmd.cmd == "version" {
			if c.X.VersionReceived {
				println("VersionAgain from", c.ConnID, c.PeerAddr.Ip(), c.Node.Agent)
				c.Misbehave("VersionAgain", 1000/10)
				break
			}
			c.X.VersionReceived = true

			er := c.HandleVersion(cmd.pl)
			if er != nil {
				println("version msg error:", er.Error())
				c.Disconnect("Version:" + er.Error())
				break
			}
			if common.FLAG.Log {
				f, _ := os.OpenFile("conn_log.txt", os.O_CREATE|os.O_RDWR|os.O_APPEND, 0660);
				if f!=nil {
					fmt.Fprintf(f, "%s: New connection. ID:%d  Incomming:%t  Addr:%s  Version:%d  Services:0x%x  Agent:%s\n",
						time.Now().Format("2006-01-02 15:04:05"), c.ConnID, c.X.Incomming,
						c.PeerAddr.Ip(), c.Node.Version, c.Node.Services, c.Node.Agent)
					f.Close()
				}
			}
			if c.Node.DoNotRelayTxs {
				c.DoS("SPV")
				break
			}
			if c.Node.Version >= 70012 {
				c.SendRawMsg("sendheaders", nil)
				if c.Node.Version >= 70013 {
					if common.CFG.TXPool.FeePerByte!=0 {
						var pl [8]byte
						binary.LittleEndian.PutUint64(pl[:], 1000*common.CFG.TXPool.FeePerByte)
						c.SendRawMsg("feefilter", pl[:])
					}
					if c.Node.Version >= 70014 {
						if (c.Node.Services&SERVICE_SEGWIT)==0 {
							// if the node does not support segwit, request compact blocks
							// only if we have not achieved he segwit enforcement moment
							if common.BlockChain.Consensus.Enforce_SEGWIT==0 ||
								common.Last.BlockHeight() < common.BlockChain.Consensus.Enforce_SEGWIT {
								c.SendRawMsg("sendcmpct", []byte{0x01,0x01,0x00,0x00,0x00,0x00,0x00,0x00,0x00})
							}
						} else {
							c.SendRawMsg("sendcmpct", []byte{0x01,0x02,0x00,0x00,0x00,0x00,0x00,0x00,0x00})
						}
					}
				}
			}
			c.PeerAddr.Services = c.Node.Services
			c.PeerAddr.Save()
			if common.IsListenTCP() {
				c.SendOwnAddr()
			}
			continue
		}

		switch cmd.cmd {
			case "inv":
				c.ProcessInv(cmd.pl)

			case "tx":
				if common.CFG.TXPool.Enabled {
					c.ParseTxNet(cmd.pl)
				}

			case "addr":
				c.ParseAddr(cmd.pl)

			case "block": //block received
				netBlockReceived(c, cmd.pl)

			case "getblocks":
				c.GetBlocks(cmd.pl)

			case "getdata":
				c.ProcessGetData(cmd.pl)

			case "getaddr":
				if !c.X.GetAddrDone {
					c.SendAddr()
					c.X.GetAddrDone = true
				} else {
					c.Mutex.Lock()
					c.counters["SecondGetAddr"]++
					c.Mutex.Unlock()
					if c.Misbehave("SecondGetAddr", 1000/20) {
						break
					}
				}

			case "ping":
				re := make([]byte, len(cmd.pl))
				copy(re, cmd.pl)
				c.SendRawMsg("pong", re)

			case "pong":
				if c.PingInProgress==nil {
					common.CountSafe("PongUnexpected")
				} else if bytes.Equal(cmd.pl, c.PingInProgress) {
					c.HandlePong()
				} else {
					common.CountSafe("PongMismatch")
				}

			case "getheaders":
				c.GetHeaders(cmd.pl)

			case "notfound":
				common.CountSafe("NotFound")

			case "headers":
				if c.HandleHeaders(cmd.pl) > 0 {
					c.sendGetHeaders()
				}

			case "sendheaders":
				c.Node.SendHeaders = true

			case "feefilter":
				if len(cmd.pl) >= 8 {
					c.X.MinFeeSPKB = int64(binary.LittleEndian.Uint64(cmd.pl[:8]))
					//println(c.PeerAddr.Ip(), c.Node.Agent, "feefilter", c.X.MinFeeSPKB)
				}

			case "sendcmpct":
				if len(cmd.pl)>=9 {
					version := binary.LittleEndian.Uint64(cmd.pl[1:9])
					if version > c.Node.SendCmpctVer {
						//println(c.ConnID, "sendcmpct", cmd.pl[0])
						c.Node.SendCmpctVer = version
						c.Node.HighBandwidth = cmd.pl[0]==1
					} else {
						c.Mutex.Lock()
						c.counters[fmt.Sprint("SendCmpctV", version)]++
						c.Mutex.Unlock()
					}
				} else {
					common.CountSafe("SendCmpctErr")
					if len(cmd.pl)!=5 {
						println(c.ConnID, c.PeerAddr.Ip(), c.Node.Agent, "sendcmpct", hex.EncodeToString(cmd.pl))
					}
				}

			case "cmpctblock":
				c.ProcessCmpctBlock(cmd.pl)

			case "getblocktxn":
				c.ProcessGetBlockTxn(cmd.pl)
				//println(c.ConnID, c.PeerAddr.Ip(), c.Node.Agent, "getblocktxn", hex.EncodeToString(cmd.pl))

			case "blocktxn":
				c.ProcessBlockTxn(cmd.pl)
				//println(c.ConnID, c.PeerAddr.Ip(), c.Node.Agent, "blocktxn", hex.EncodeToString(cmd.pl))

			default:
				if common.DebugLevel>0 {
					println(cmd.cmd, "from", c.PeerAddr.Ip())
				}
		}
	}

	c.Mutex.Lock()
	MutexRcv.Lock()
	for k, _ := range c.GetBlockInProgress {
		if rec, ok := BlocksToGet[k] ; ok {
			rec.InProgress--
		} else {
			//println("ERROR! Block", bip.hash.String(), "in progress, but not in BlocksToGet")
		}
	}
	MutexRcv.Unlock()

	ban := c.banit
	c.Mutex.Unlock()
	if ban {
		c.PeerAddr.Ban()
		common.CountSafe("PeersBanned")
	} else if c.X.Incomming && !c.X.IsSpecial.Get() {
		HammeringMutex.Lock()
		RecentlyDisconencted[c.PeerAddr.NetAddr.Ip4] = time.Now()
		HammeringMutex.Unlock()
	}
	if common.DebugLevel!=0 {
		println("Disconnected from", c.PeerAddr.Ip())
	}
	c.Conn.Close()
}
