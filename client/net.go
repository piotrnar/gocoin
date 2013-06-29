package main

import (
	"fmt"
	"net"
	"time"
	"bytes"
	"strings"
	"math/rand"
	"github.com/piotrnar/gocoin/btc"
)

const measure_block_timing = true


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

	if c.GetBlockInProgress!=nil {
		if c.GetBlockHeaderGot {
			if time.Now().After(c.GetBlockInProgressAt.Add(GetBlockPayloadTimeout)) {
				CountSafe("GetBlockPayloadTout")
				c.GetBlockInProgress = nil
			}
		} else {
			if c.recv.hdr_len==24 && strings.TrimRight(string(c.recv.hdr[4:16]), "\000")=="block" &&
				len(c.recv.dat)>=80 && c.recv.datlen>=80 {
				var hash [32]byte
				btc.ShaHash(c.recv.dat[:80], hash[:])
				if c.GetBlockInProgress.Hash == hash {
					if measure_block_timing {
						pb := pendingBlocks[c.GetBlockInProgress.BIdx()]
						if pb==nil {
							panic("wtf??? why not pending?")
						} else {
							println("New Block", c.GetBlockInProgress.String(), "header after",
								time.Now().Sub(pb.noticed).String(), "in", c.PeerAddr.Ip())
						}
						ui_show_prompt()
					}
					c.GetBlockHeaderGot = true
				} else {
					println("Received header for a different block")
					println("Expected:", c.GetBlockInProgress.String())
					println("Received:", btc.NewUint256(hash[:]).String())
					ui_show_prompt()
				}
			} else {
				if time.Now().After(c.GetBlockInProgressAt.Add(GetBlockHeaderTimeout)) {
					CountSafe("GetBlockHeaderTout")
					c.GetBlockInProgress = nil
				}
			}
		}
	}

	// Need to send getdata...?
	if c.GetBlockInProgress==nil {
		if tmp := blockDataNeeded(); tmp != nil {
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
			time.Sleep(250e6)
		}
	}
}

// General network process (i.e. for establishing outgoing connections)
func network_process() {
	if CFG.ListenTCP {
		if CFG.ConnectOnly=="" {
			go tcp_server()
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
				do_network(adrs[rand.Int31n(int32(len(adrs)))])
				continue // do not sleep
			}

			if CFG.ConnectOnly=="" && dbg>0 {
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


// Process that handles communication with a single peer
func (c *oneConnection) Run() {
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
				if CFG.ListenTCP {
					c.SendOwnAddr()
				}

			case "inv":
				c.ProcessInv(cmd.pl)

			case "tx":
				if CFG.TXRouting.Enabled {
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
