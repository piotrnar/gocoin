package network

import (
	"fmt"
	"net"
	"time"
	"bytes"
	"math/rand"
	"sync/atomic"
	"github.com/piotrnar/gocoin/client/common"
)


func (c *OneConnection) Tick() {
	c.Mutex.Lock()
	c.TicksCnt++
	c.Mutex.Unlock()

	// Disconnect and ban useless peers (sych that don't send invs)
	if c.InvsRecieved==0 && c.ConnectedAt.Add(15*time.Minute).Before(time.Now()) {
		common.CountSafe("NetUselessPeer")
		c.DoS()
		return
	}

	// Check no-data timeout
	if c.LastDataGot.Add(NoDataTimeout).Before(time.Now()) {
		c.Disconnect()
		common.CountSafe("NetNodataTout")
		if common.DebugLevel>0 {
			println(c.PeerAddr.Ip(), "no data for", NoDataTimeout/time.Second, "seconds - disconnect")
		}
		return
	}

	if c.Send.Buf != nil {
		n, e := common.SockWrite(c.NetConn, c.Send.Buf)
		if n > 0 {
			c.Mutex.Lock()
			c.Send.LastSent = time.Now()
			c.BytesSent += uint64(n)
			if n >= len(c.Send.Buf) {
				c.Send.Buf = nil
			} else {
				tmp := make([]byte, len(c.Send.Buf)-n)
				copy(tmp, c.Send.Buf[n:])
				c.Send.Buf = tmp
			}
			c.Mutex.Unlock()
		} else if time.Now().After(c.Send.LastSent.Add(AnySendTimeout)) {
			common.CountSafe("PeerSendTimeout")
			c.Disconnect()
		} else if e != nil {
			if common.DebugLevel > 0 {
				println(c.PeerAddr.Ip(), "Connection Broken during send")
			}
			c.Disconnect()
		}
		return
	}

	if !c.VerackReceived {
		// If we have no ack, do nothing more.
		return
	}

	// Ask node for new addresses...?
	if time.Now().After(c.NextGetAddr) {
		if PeerDB.Count() > common.MaxPeersNeeded {
			// If we have a lot of peers, do not ask for more, to save bandwidth
			common.CountSafe("AddrEnough")
		} else {
			common.CountSafe("AddrWanted")
			c.SendRawMsg("getaddr", nil)
		}
		c.NextGetAddr = time.Now().Add(AskAddrsEvery)
		return
	}

	// Need to send some invs...?
	if c.SendInvs() {
		return
	}

	// Timeout getdata for blocks in progress, so the map does not grow to infinity
	for k, v := range c.GetBlockInProgress {
		if time.Now().After(v.start.Add(GetBlockTimeout)) {
			common.CountSafe("GetBlockTimeout")
			c.Mutex.Lock()
			delete(c.GetBlockInProgress, k)
			c.Mutex.Unlock()
		}
	}

	// Need to send getblocks...?
	if len(c.GetBlockInProgress)==0 && c.getblocksNeeded() {
		return
	}

	// Ping if we dont do anything
	c.TryPing()
}


func DoNetwork(ad *onePeer) {
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
	OpenCons[ad.UniqID()] = conn
	OutConsActive++
	Mutex_net.Unlock()
	go func() {
		conn.NetConn, e = net.DialTimeout("tcp4", fmt.Sprintf("%d.%d.%d.%d:%d",
			ad.Ip4[0], ad.Ip4[1], ad.Ip4[2], ad.Ip4[3], ad.Port), TCPDialTimeout)
		if e == nil {
			conn.ConnectedAt = time.Now()
			if common.DebugLevel>0 {
				println("Connected to", ad.Ip())
			}
			conn.Run()
		} else {
			if common.DebugLevel>0 {
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
	next_drop_slowest time.Time
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

	for common.CFG.Net.ListenTCP {
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
					fmt.Println("Incomming connection from", tc.RemoteAddr().String())
				}
				ad, e := NewIncommingPeer(tc.RemoteAddr().String())
				if e == nil {
					// Hammering protection
					HammeringMutex.Lock()
					ti, ok := RecentlyDisconencted[ad.NetAddr.Ip4]
					HammeringMutex.Unlock()
					if ok && time.Now().Sub(ti) < HammeringMinReconnect {
						//println(ad.Ip(), "is hammering within", time.Now().Sub(ti).String())
						common.CountSafe("InConnHammer")
						ad.Ban()
						terminate = true
					}

					if !terminate {
						// Incomming IP passed all the initial checks - talk to it
						conn := NewConnection(ad)
						conn.ConnectedAt = time.Now()
						conn.Incomming = true
						conn.NetConn = tc
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
						println("NewIncommingPeer:", e.Error())
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
		if c.Incomming {
			c.Disconnect()
		}
	}
	TCPServerStarted = false
	Mutex_net.Unlock()
	//fmt.Println("TCP server stopped")
}


func NetworkTick() {
	common.CountSafe("NetTicks")

	if common.CFG.Net.ListenTCP {
		if !TCPServerStarted {
			TCPServerStarted = true
			go tcp_server()
		}
	}

	Mutex_net.Lock()
	conn_cnt := OutConsActive
	Mutex_net.Unlock()

	if next_drop_slowest.IsZero() {
		next_drop_slowest = time.Now().Add(DropSlowestEvery)
	} else if conn_cnt >= atomic.LoadUint32(&common.CFG.Net.MaxOutCons) {
		// Having max number of outgoing connections, check to drop the slowest one
		if time.Now().After(next_drop_slowest) {
			drop_slowest_peer()
			next_drop_slowest = time.Now().Add(DropSlowestEvery)
		}
	}

	// hammering protection - expire recently disconnected
	if next_clean_hammers.IsZero() {
		next_clean_hammers = time.Now().Add(HammeringMinReconnect)
	} else if time.Now().After(next_clean_hammers) {
		HammeringMutex.Lock()
		for k, t := range RecentlyDisconencted {
			if time.Now().Sub(t) >= HammeringMinReconnect {
				delete(RecentlyDisconencted, k)
			}
		}
		HammeringMutex.Unlock()
		next_clean_hammers = time.Now().Add(HammeringMinReconnect)
	}

	for conn_cnt < atomic.LoadUint32(&common.CFG.Net.MaxOutCons) {
		adrs := GetBestPeers(16, true)
		if len(adrs)==0 {
			common.LockCfg()
			if common.CFG.ConnectOnly=="" && common.DebugLevel>0 {
				println("no new peers", len(OpenCons), conn_cnt)
			}
			common.UnlockCfg()
			break
		}
		DoNetwork(adrs[rand.Int31n(int32(len(adrs)))])
		Mutex_net.Lock()
		conn_cnt = OutConsActive
		Mutex_net.Unlock()
	}
}


// Process that handles communication with a single peer
func (c *OneConnection) Run() {
	c.SendVersion()

	c.Mutex.Lock()
	c.LastDataGot = time.Now()
	c.NextBlocksAsk = time.Now() // ask for blocks ASAP
	c.NextGetAddr = time.Now()  // do getaddr ~10 seconds from now
	c.NextPing = time.Now().Add(5*time.Second)  // do first ping ~5 seconds from now
	c.Mutex.Unlock()

	for !c.IsBroken() {
		c.Mutex.Lock()
		c.LoopCnt++
		c.Mutex.Unlock()

		cmd := c.FetchMessage()
		if c.IsBroken() {
			break
		}

		// Timeout ping in progress
		if c.PingInProgress!=nil && time.Now().After(c.LastPingSent.Add(PingTimeout)) {
			if common.DebugLevel > 0 {
				println(c.PeerAddr.Ip(), "ping timeout")
			}
			common.CountSafe("PingTimeout")
			c.HandlePong()  // this will set LastPingSent to nil
		}

		if cmd==nil {
			c.Tick()
			continue
		}

		c.Mutex.Lock()
		c.LastDataGot = time.Now()
		c.LastCmdRcvd = cmd.cmd
		c.LastBtsRcvd = uint32(len(cmd.pl))
		c.Mutex.Unlock()

		c.PeerAddr.Alive()
		if common.DebugLevel<0 {
			fmt.Println(c.PeerAddr.Ip(), "->", cmd.cmd, len(cmd.pl))
		}

		if c.Send.Buf != nil && len(c.Send.Buf) > SendBufSizeHoldOn {
			common.CountSafe("hold_"+cmd.cmd)
			common.CountSafeAdd("hbts_"+cmd.cmd, uint64(len(cmd.pl)))
			continue
		}

		common.CountSafe("rcvd_"+cmd.cmd)
		common.CountSafeAdd("rbts_"+cmd.cmd, uint64(len(cmd.pl)))

		switch cmd.cmd {
			case "version":
				er := c.HandleVersion(cmd.pl)
				if er != nil {
					println("version:", er.Error())
					c.Disconnect()
				}

			case "verack":
				c.VerackReceived = true
				if common.CFG.Net.ListenTCP {
					c.SendOwnAddr()
				}

			case "inv":
				c.ProcessInv(cmd.pl)

			case "tx":
				if common.CFG.TXPool.Enabled {
					c.ParseTxNet(cmd.pl)
				}

			case "addr":
				ParseAddr(cmd.pl)

			case "block": //block received
				netBlockReceived(c, cmd.pl)

			case "getblocks":
				c.ProcessGetBlocks(cmd.pl)

			case "getdata":
				c.ProcessGetData(cmd.pl)

			case "getaddr":
				c.SendAddr()

			case "alert":
				c.HandleAlert(cmd.pl)

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

			case "notfound":
				common.CountSafe("NotFound")

			default:
				if common.DebugLevel>0 {
					println(cmd.cmd, "from", c.PeerAddr.Ip())
				}
		}
	}
	c.Mutex.Lock()
	ban := c.banit
	c.Mutex.Unlock()
	if ban {
		c.PeerAddr.Ban()
		common.CountSafe("PeersBanned")
	} else if c.Incomming {
		HammeringMutex.Lock()
		RecentlyDisconencted[c.PeerAddr.NetAddr.Ip4] = time.Now()
		HammeringMutex.Unlock()
	}
	if common.DebugLevel>0 {
		println("Disconnected from", c.PeerAddr.Ip())
	}
	c.NetConn.Close()
}
