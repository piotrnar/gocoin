package main

import (
	"fmt"
	"net"
	"time"
	"bytes"
	"math/rand"
)


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
		n, e := SockWrite(c.NetConn, c.send.buf[c.send.sofar:])
		if n > 0 {
			c.send.lastSent = time.Now()
			c.BytesSent += uint64(n)
			c.send.sofar += n
			if c.send.sofar >= len(c.send.buf) {
				c.send.buf = nil
				c.send.sofar = 0
			}
		} else if time.Now().After(c.send.lastSent.Add(AnySendTimeout)) {
			CountSafe("PeerSendTimeout")
			c.Broken = true
		} else if e != nil {
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

	// Need to send some invs...?
	if c.SendInvs() {
		return
	}

	// Timeout getdata for blocks in progress, so the map does not grow to infinity
	for k, v := range c.GetBlockInProgress {
		if time.Now().After(v.start.Add(GetBlockTimeout)) {
			CountSafe("GetBlockTimeout")
			delete(c.GetBlockInProgress, k)
		}
	}

	// Need to send getblocks...?
	if len(c.GetBlockInProgress)==0 && c.getblocksNeeded() {
		return
	}

	// Ping if we dont do anything
	c.TryPing()
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
			conn.Run()
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


var (
	tcp_server_started bool
	next_drop_slowest time.Time
)


// TCP server
func tcp_server() {
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

	for CFG.ListenTCP {
		CountSafe("NetServerLoops")
		if InConsActive < MaxInCons {
			lis.SetDeadline(time.Now().Add(time.Second))
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
							conn.Run()
							mutex.Lock()
							delete(openCons, ad.UniqID())
							InConsActive--
							mutex.Unlock()
						}()
					}
				} else {
					if dbg>0 {
						println("NewIncommingPeer:", e.Error())
					}
					CountSafe("InConnRefused")
					tc.Close()
				}
			}
		} else {
			time.Sleep(1e9)
		}
	}
	mutex.Lock()
	for _, c := range openCons {
		if c.Incomming {
			c.Broken = true
		}
	}
	mutex.Unlock()
	fmt.Println("TCP server stopped")
	tcp_server_started = false
}


func network_tick() {
	CountSafe("NetTicks")

	if CFG.ListenTCP {
		if !tcp_server_started {
			tcp_server_started = true
			go tcp_server()
		}
	}

	mutex.Lock()
	conn_cnt := OutConsActive
	mutex.Unlock()

	if next_drop_slowest.IsZero() {
		next_drop_slowest = time.Now().Add(DropSlowestEvery)
	} else if conn_cnt >= MaxOutCons {
		// Having max number of outgoing connections, check to drop the slowest one
		if time.Now().After(next_drop_slowest) {
			drop_slowest_peer()
			next_drop_slowest = time.Now().Add(DropSlowestEvery)
		}
	}

	for conn_cnt < MaxOutCons {
		adrs := GetBestPeers(16, true)
		if len(adrs)==0 {
			if CFG.ConnectOnly=="" && dbg>0 {
				println("no new peers", len(openCons), conn_cnt)
			}
			break
		}
		do_network(adrs[rand.Int31n(int32(len(adrs)))])
		mutex.Lock()
		conn_cnt = OutConsActive
		mutex.Unlock()
	}
}


// Process that handles communication with a single peer
func (c *oneConnection) Run() {
	c.SendVersion()

	c.LastDataGot = time.Now()
	c.NextBlocksAsk = time.Now() // ask for blocks ASAP
	c.NextGetAddr = time.Now()  // do getaddr ~10 seconds from now
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
				}

			case "verack":
				c.VerackReceived = true
				if CFG.ListenTCP {
					c.SendOwnAddr()
				}

			case "inv":
				c.ProcessInv(cmd.pl)

			case "tx":
				if CFG.TXPool.Enabled {
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
		CountSafe("PeersBanned")
	}
	if dbg>0 {
		println("Disconnected from", c.PeerAddr.Ip())
	}
	c.NetConn.Close()
}
