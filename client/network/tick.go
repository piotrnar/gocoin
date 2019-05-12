package network

import (
	"bufio"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"github.com/piotrnar/gocoin/client/common"
	"github.com/piotrnar/gocoin/lib/btc"
	"github.com/piotrnar/gocoin/lib/others/peersdb"
	"math/rand"
	"net"
	"os"
	"runtime/debug"
	"strings"
	"time"
)

var (
	TCPServerStarted   bool
	next_drop_peer     time.Time
	next_clean_hammers time.Time

	NextConnectFriends time.Time = time.Now()
	AuthPubkeys        [][]byte

	GetMPInProgressTicket = make(chan bool, 1)
)

// call with unlocked c.Mutex
func (c *OneConnection) ExpireHeadersAndGetData(now *time.Time, curr_ping_cnt uint64) {
	var disconnect string

	c.Mutex.Lock()
	/*if c.X.Debug {
		println(c.ConnID, "- ExpireHeadersAndGetData", curr_ping_cnt, c.X.GetHeadersSentAtPingCnt, c.X.GetHeadersInProgress, len(c.GetBlockInProgress))
	}*/

	if c.X.GetHeadersInProgress {
		var err string
		if curr_ping_cnt > c.X.GetHeadersSentAtPingCnt {
			err = "GetHeadersPong"
		} else if now != nil && now.After(c.X.GetHeadersTimeOutAt) {
			err = "GetHeadersTimeout"
		}
		if err != "" {
			// GetHeaders timeout accured
			c.X.GetHeadersInProgress = false
			c.X.LastHeadersEmpty = true
			c.X.AllHeadersReceived = true
			common.CountSafe(err)
			disconnect = err
		}
	}
	c.Mutex.Unlock()

	// never lock network.MutexRcv within (*OneConnection).Mutex as it can cause a deadlock
	MutexRcv.Lock()
	c.Mutex.Lock()
	for k, v := range c.GetBlockInProgress {
		if curr_ping_cnt > v.SentAtPingCnt {
			common.CountSafe("BlockInprogNotfound")
			c.counters["BlockNotFound"]++
		} else if now != nil && now.After(v.start.Add(5*time.Minute)) {
			common.CountSafe("BlockInprogTimeout")
			c.counters["BlockTimeout"]++
		} else {
			continue
		}
		c.X.BlocksExpired++
		delete(c.GetBlockInProgress, k)
		if bip, ok := BlocksToGet[k]; ok {
			bip.InProgress--
		}
		if now == nil {
			disconnect = "BlockDlPongExp"
		} else {
			disconnect = "BlockDlTimeout"
		}
	}
	c.Mutex.Unlock()
	MutexRcv.Unlock()

	if disconnect != "" {
		if c.X.IsSpecial {
			common.CountSafe("Spec"+disconnect)
			c.counters[disconnect]++
		} else {
			c.Disconnect(disconnect)
		}
	}
}

// Call this once a minute
func (c *OneConnection) Maintanence(now time.Time) {
	// Expire GetBlockInProgress after five minutes, if they are not in BlocksToGet
	c.ExpireHeadersAndGetData(&now, 0)

	// Expire BlocksReceived after two days
	c.Mutex.Lock()
	if len(c.blocksreceived) > 0 {
		var i int
		for i = 0; i < len(c.blocksreceived); i++ {
			if c.blocksreceived[i].Add(common.GetDuration(&common.BlockExpireEvery)).After(now) {
				break
			}
			common.CountSafe("BlksRcvdExpired")
		}
		if i > 0 {
			//println(c.ConnID, "expire", i, "block(s)")
			c.blocksreceived = c.blocksreceived[i:]
		}
	}
	c.Mutex.Unlock()
}

func (c *OneConnection) Tick(now time.Time) {
	if !c.X.VersionReceived {
		// Wait only certain amount of time for the version message
		if c.X.ConnectedAt.Add(VersionMsgTimeout).Before(now) {
			c.Disconnect("VersionTimeout")
			common.CountSafe("NetVersionTout")
			return
		}
		// If we have no ack, do nothing more.
		return
	}

	if common.GetBool(&common.BlockChainSynchronized) {
		// See if to send "getmp" command
		select {
		case GetMPInProgressTicket <- true:
			// ticket received - check for the request...
			if len(c.GetMP) == 0 || c.SendGetMP() != nil {
				// no request for "getmp" here or sending failed - clear the global flag/channel
				_ = <-GetMPInProgressTicket
			}
		default:
			// failed to get the ticket - just do nothing
		}
	}

	// Tick the recent transactions counter
	if now.After(c.txsNxt) {
		c.Mutex.Lock()
		if len(c.txsCha) == cap(c.txsCha) {
			tmp := <-c.txsCha
			c.X.TxsReceived -= tmp
		}
		c.txsCha <- c.txsCur
		c.txsCur = 0
		c.txsNxt = c.txsNxt.Add(TxsCounterPeriod)
		c.Mutex.Unlock()
	}

	if mfpb := common.MinFeePerKB(); mfpb != c.X.LastMinFeePerKByte {
		c.X.LastMinFeePerKByte = mfpb
		if c.Node.Version >= 70013 {
			c.SendFeeFilter()
		}
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

	if !c.X.GetHeadersInProgress && !c.X.AllHeadersReceived && len(c.GetBlockInProgress) == 0 {
		c.Mutex.Unlock()
		c.sendGetHeaders()
		return // new headers requested
	}

	if c.X.AllHeadersReceived {
		if !c.X.GetBlocksDataNow && now.After(c.nextGetData) {
			c.X.GetBlocksDataNow = true
		}
		if c.X.GetBlocksDataNow && len(c.GetBlockInProgress) == 0 {
			c.X.GetBlocksDataNow = false
			c.Mutex.Unlock()
			c.GetBlockData()
			return // block data requested
		}
	}

	c.Mutex.Unlock()

	// nothing requested - free to ping..
	c.TryPing()
}

func DoNetwork(ad *peersdb.PeerAddr) {
	conn := NewConnection(ad)
	Mutex_net.Lock()
	if _, ok := OpenCons[ad.UniqID()]; ok {
		common.CountSafe("ConnectingAgain")
		Mutex_net.Unlock()
		return
	}
	if ad.Friend || ad.Manual {
		conn.MutexSetBool(&conn.X.IsSpecial, true)
	}
	OpenCons[ad.UniqID()] = conn
	OutConsActive++
	Mutex_net.Unlock()
	go func() {
		var con net.Conn
		var e error
		con_done := make(chan bool, 1)

		go func(addr string) {
			// we do net.Dial() in paralell routine, so we can abort quickly upon request
			con, e = net.DialTimeout("tcp4", addr, TCPDialTimeout)
			con_done <- true
		}(fmt.Sprintf("%d.%d.%d.%d:%d", ad.Ip4[0], ad.Ip4[1], ad.Ip4[2], ad.Ip4[3], ad.Port))

		for {
			select {
			case <-con_done:
				if e == nil {
					Mutex_net.Lock()
					conn.Conn = con
					conn.X.ConnectedAt = time.Now()
					Mutex_net.Unlock()
					conn.Run()
				}
			case <-time.After(10 * time.Millisecond):
				if !conn.IsBroken() {
					continue
				}
			}
			break
		}

		Mutex_net.Lock()
		delete(OpenCons, ad.UniqID())
		OutConsActive--
		Mutex_net.Unlock()
		ad.Dead()
	}()
}

// TCP server
func tcp_server() {
	ad, e := net.ResolveTCPAddr("tcp4", fmt.Sprint("0.0.0.0:", common.DefaultTcpPort()))
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
		if ica < common.GetUint32(&common.CFG.Net.MaxInCons) {
			lis.SetDeadline(time.Now().Add(100 * time.Millisecond))
			tc, e := lis.AcceptTCP()
			if e == nil && common.IsListenTCP() {
				var terminate bool

				// set port to default, for incmming connections
				ad, e := peersdb.NewPeerFromString(tc.RemoteAddr().String(), true)
				if e == nil {
					// Hammering protection
					HammeringMutex.Lock()
					if rd := RecentlyDisconencted[ad.NetAddr.Ip4]; rd != nil {
						rd.Count++
						terminate = rd.Count > HammeringMaxAllowedCount
					}
					HammeringMutex.Unlock()

					if terminate {
						common.CountSafe("BanHammerIn")
						ad.Ban()
					} else {
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
							go func() {
								conn.Run()
								Mutex_net.Lock()
								delete(OpenCons, ad.UniqID())
								InConsActive--
								Mutex_net.Unlock()
							}()
						}
					}
				} else {
					common.CountSafe("InConnRefused")
					terminate = true
				}

				// had any error occured - close teh TCP connection
				if terminate {
					tc.Close()
				}
			}
		} else {
			time.Sleep(1e8)
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

func ConnectFriends() {
	common.CountSafe("ConnectFriends")

	f, _ := os.Open(common.GocoinHomeDir + "friends.txt")
	if f == nil {
		return
	}
	defer f.Close()

	AuthPubkeys = nil
	friend_ids := make(map[uint64]bool)

	rd := bufio.NewReader(f)
	if rd != nil {
		for {
			ln, _, er := rd.ReadLine()
			if er != nil {
				break
			}
			ls := strings.SplitN(strings.Trim(string(ln), "\r\n\t"), " ", 2)
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
					curr.Mutex.Lock()
					curr.PeerAddr.Friend = true
					curr.X.IsSpecial = true
					curr.Mutex.Unlock()
				}
				friend_ids[ad.UniqID()] = true
				continue
			}
			pk := btc.Decodeb58(ls[0])
			if len(pk) == 33 {
				AuthPubkeys = append(AuthPubkeys, pk)
				//println("Using pubkey:", hex.EncodeToString(pk))
			}
		}
	}

	// Unmark those that are not longer friends
	Mutex_net.Lock()
	for _, v := range OpenCons {
		v.Lock()
		if v.PeerAddr.Friend && !friend_ids[v.PeerAddr.UniqID()] {
			v.PeerAddr.Friend = false
			if !v.PeerAddr.Manual {
				v.X.IsSpecial = false
			}
		}
		v.Unlock()
	}
	Mutex_net.Unlock()
}

func NetworkTick() {
	if common.IsListenTCP() {
		if !TCPServerStarted {
			TCPServerStarted = true
			go tcp_server()
		}
	}

	now := time.Now()

	// Push GetHeaders if not in progress
	Mutex_net.Lock()
	var cnt_headers_in_progress int
	var max_headers_got_cnt int
	var _v *OneConnection
	for _, v := range OpenCons {
		v.Mutex.Lock() // TODO: Sometimes it might hang here - check why!!
		if !v.X.AllHeadersReceived || v.X.GetHeadersInProgress {
			cnt_headers_in_progress++
		} else if !v.X.LastHeadersEmpty {
			if _v == nil || v.X.TotalNewHeadersCount > max_headers_got_cnt {
				max_headers_got_cnt = v.X.TotalNewHeadersCount
				_v = v
			}
		}
		v.Mutex.Unlock()
	}
	conn_cnt := OutConsActive
	Mutex_net.Unlock()

	if cnt_headers_in_progress == 0 {
		if _v != nil {
			common.CountSafe("GetHeadersPush")
			/*println("No headers_in_progress, so take it from", _v.ConnID,
			_v.X.TotalNewHeadersCount, _v.X.LastHeadersEmpty)*/
			_v.Mutex.Lock()
			if _v.X.Debug {
				println(_v.ConnID, "- GetHeadersPush")
			}
			_v.X.AllHeadersReceived = false
			_v.Mutex.Unlock()
		} else {
			common.CountSafe("GetHeadersNone")
		}
	}

	if common.CFG.DropPeers.DropEachMinutes != 0 {
		if next_drop_peer.IsZero() {
			next_drop_peer = now.Add(common.GetDuration(&common.DropSlowestEvery))
		} else if now.After(next_drop_peer) {
			if drop_worst_peer() {
				next_drop_peer = now.Add(common.GetDuration(&common.DropSlowestEvery))
			} else {
				// If no peer dropped this time, try again sooner
				next_drop_peer = now.Add(common.GetDuration(&common.DropSlowestEvery) >> 2)
			}
		}
	}

	// hammering protection - expire recently disconnected, but not more often than once a minute
	if next_clean_hammers.IsZero() {
		next_clean_hammers = now.Add(HammeringExpirePeriod)
	} else if now.After(next_clean_hammers) {
		HammeringMutex.Lock()
		for k, t := range RecentlyDisconencted {
			if now.Sub(t.Time) >= HammeringMinReconnect {
				delete(RecentlyDisconencted, k)
			}
		}
		HammeringMutex.Unlock()
		next_clean_hammers = now.Add(HammeringExpirePeriod)
	}

	// Connect friends
	Mutex_net.Lock()
	if now.After(NextConnectFriends) {
		Mutex_net.Unlock()
		ConnectFriends()
		Mutex_net.Lock()
		NextConnectFriends = now.Add(15 * time.Minute)
	}
	Mutex_net.Unlock()

	for conn_cnt < common.GetUint32(&common.CFG.Net.MaxOutCons) {
		var segwit_conns uint32
		if common.CFG.Net.MinSegwitCons > 0 {
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

		adrs := peersdb.GetBestPeers(128, func(ad *peersdb.PeerAddr) bool {
			if segwit_conns < common.CFG.Net.MinSegwitCons && (ad.Services&SERVICE_SEGWIT) == 0 {
				return true
			}
			return ConnectionActive(ad)
		})
		if len(adrs) == 0 && segwit_conns < common.CFG.Net.MinSegwitCons {
			// we have only non-segwit peers in the database - take them
			adrs = peersdb.GetBestPeers(128, func(ad *peersdb.PeerAddr) bool {
				return ConnectionActive(ad)
			})
		}
		if len(adrs) == 0 {
			common.LockCfg()
			common.UnlockCfg()
			break
		}
		DoNetwork(adrs[rand.Int31n(int32(len(adrs)))])
		Mutex_net.Lock()
		conn_cnt = OutConsActive
		Mutex_net.Unlock()
	}

	if expireTxsNow {
		ExpireTxs()
	} else if now.After(lastTxsExpire.Add(time.Minute)) {
		expireTxsNow = true
	}
}

func (c *OneConnection) SendFeeFilter() {
	var pl [8]byte
	binary.LittleEndian.PutUint64(pl[:], c.X.LastMinFeePerKByte)
	c.SendRawMsg("feefilter", pl[:])
}

func (c *OneConnection) SendAuth() {
	rnd := make([]byte, 32)
	copy(rnd, c.Node.Nonce[:])
	r, s, er := btc.EcdsaSign(common.SecretKey, rnd)
	if er != nil {
		println(er.Error())
		return
	}
	var sig btc.Signature
	sig.R.Set(r)
	sig.S.Set(s)
	c.SendRawMsg("auth", sig.Bytes())
}

func (c *OneConnection) AuthRvcd(pl []byte) {
	if c.X.AuthMsgGot > 0 {
		c.DoS("AuthMsgCnt") // Only allow one auth message per connection (DoS prevention)
		return
	}
	c.X.AuthMsgGot++
	rnd := make([]byte, 32)
	copy(rnd, nonce[:])
	for _, pub := range AuthPubkeys {
		if btc.EcdsaVerify(pub, pl, rnd) {
			c.X.Authorized = true
			c.SendRawMsg("authack", nil)
			return
		}
	}
	c.X.Authorized = false
}

// call it upon receiving "getmpdone" message or when the peer disconnects
func (c *OneConnection) GetMPDone(pl []byte) {
	if len(c.GetMP) > 0 {
		if len(pl) != 1 || pl[0] == 0 || c.SendGetMP() != nil {
			_ = <-c.GetMP
			if len(GetMPInProgressTicket) > 0 {
				_ = <-GetMPInProgressTicket
			}
		}
	}
}

// Process that handles communication with a single peer
func (c *OneConnection) Run() {
	defer func() {
		if r := recover(); r != nil {
			err, ok := r.(error)
			if !ok {
				err = fmt.Errorf("pkg: %v", r)
			}
			fmt.Println()
			fmt.Println()
			fmt.Println("********************** THIS SHOULD NOT HAPPEN **********************")
			fmt.Println("Please report by sending email to piotr@gocoin.pl")
			fmt.Println("or by logging new issue at https://github.com/piotrnar/gocoin/issues")
			fmt.Println()
			fmt.Println("Make sure to include the data below:")
			fmt.Println()
			fmt.Println(err.Error())
			fmt.Println(string(debug.Stack()))
			fmt.Println()
			fmt.Println("The node will likely malfunction now - it is advised to restart it.")
			fmt.Println("************************ END OF REPORT ****************************")
		}
	}()

	c.writing_thread_push = make(chan bool, 1)

	c.SendVersion()

	c.Mutex.Lock()
	now := time.Now()
	c.X.LastDataGot = now
	c.nextMaintanence = now.Add(time.Minute)
	c.LastPingSent = now.Add(5*time.Second - common.GetDuration(&common.PingPeerEvery)) // do first ping ~5 seconds from now

	c.txsNxt = now.Add(TxsCounterPeriod)
	c.txsCha = make(chan int, TxsCounterBufLen)

	c.Mutex.Unlock()

	next_tick := now
	next_invs := now

	c.writing_thread_done.Add(1)
	go c.writing_thread()

	for !c.IsBroken() {
		if c.IsBroken() {
			break
		}

		cmd, read_tried := c.FetchMessage()

		now = time.Now()
		if c.X.VersionReceived && now.After(next_invs) {
			c.SendInvs()
			next_invs = now.Add(InvsFlushPeriod)
		}

		if now.After(next_tick) {
			c.Tick(now)
			next_tick = now.Add(PeerTickPeriod)
		}

		if cmd == nil {
			if c.unfinished_getdata != nil && !c.SendingPaused() {
				cmd = &BCmsg{cmd:"getdata", pl:c.unfinished_getdata}
				common.CountSafe("GetDataRestored")
				goto recovered_getdata
			}

			if !read_tried {
				// it will end up here if we did not even try to read anything because of BW limit
				time.Sleep(10 * time.Millisecond)
			}
			continue
		}

		if c.X.VersionReceived {
			c.PeerAddr.Alive()
		}

		if cmd.cmd == "version" {
			if c.X.VersionReceived {
				println("VersionAgain from", c.ConnID, c.PeerAddr.Ip(), c.Node.Agent)
				c.Misbehave("VersionAgain", 1000/10)
				break
			}
			er := c.HandleVersion(cmd.pl)
			if er != nil {
				//println("version msg error:", er.Error())
				c.Disconnect("Version:" + er.Error())
				break
			}
			if common.FLAG.Log {
				f, _ := os.OpenFile("conn_log.txt", os.O_CREATE|os.O_RDWR|os.O_APPEND, 0660)
				if f != nil {
					fmt.Fprintf(f, "%s: New connection. ID:%d  Incomming:%t  Addr:%s  Version:%d  Services:0x%x  Agent:%s\n",
						time.Now().Format("2006-01-02 15:04:05"), c.ConnID, c.X.Incomming,
						c.PeerAddr.Ip(), c.Node.Version, c.Node.Services, c.Node.Agent)
					f.Close()
				}
			}
			c.X.LastMinFeePerKByte = common.MinFeePerKB()

			if c.X.IsGocoin {
				c.SendAuth()
			}

			if c.Node.Version >= 70012 {
				c.SendRawMsg("sendheaders", nil)
				if c.Node.Version >= 70013 {
					if c.X.LastMinFeePerKByte != 0 {
						c.SendFeeFilter()
					}
					if c.Node.Version >= 70014 && common.GetBool(&common.CFG.TXPool.Enabled) {
						if (c.Node.Services & SERVICE_SEGWIT) == 0 {
							// if the node does not support segwit, request compact blocks
							// only if we have not achieved the segwit enforcement moment
							if common.BlockChain.Consensus.Enforce_SEGWIT == 0 ||
								common.Last.BlockHeight() < common.BlockChain.Consensus.Enforce_SEGWIT {
								c.SendRawMsg("sendcmpct", []byte{0x01, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00})
							}
						} else {
							c.SendRawMsg("sendcmpct", []byte{0x01, 0x02, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00})
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

	recovered_getdata:
		switch cmd.cmd {
		case "inv":
			c.ProcessInv(cmd.pl)

		case "tx":
			if common.AcceptTx() {
				c.ParseTxNet(cmd.pl)
			}

		case "addr":
			c.ParseAddr(cmd.pl)

		case "block": //block received
			netBlockReceived(c, cmd.pl)
			c.X.GetBlocksDataNow = true // try to ask for more blocks

		case "getblocks":
			c.GetBlocks(cmd.pl)

		case "getdata":
			c.unfinished_getdata = nil
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
			c.HandlePong(cmd.pl)

		case "getheaders":
			c.GetHeaders(cmd.pl)

		case "notfound":
			common.CountSafe("NotFound")

		case "headers":
			if c.HandleHeaders(cmd.pl) > 0 {
				c.sendGetHeaders()
			}

		case "sendheaders":
			c.Mutex.Lock()
			c.Node.SendHeaders = true
			c.Mutex.Unlock()

		case "feefilter":
			if len(cmd.pl) >= 8 {
				c.X.MinFeeSPKB = int64(binary.LittleEndian.Uint64(cmd.pl[:8]))
				//println(c.PeerAddr.Ip(), c.Node.Agent, "feefilter", c.X.MinFeeSPKB)
			}

		case "sendcmpct":
			if len(cmd.pl) >= 9 {
				version := binary.LittleEndian.Uint64(cmd.pl[1:9])
				c.Mutex.Lock()
				if version > c.Node.SendCmpctVer {
					//println(c.ConnID, "sendcmpct", cmd.pl[0])
					c.Node.SendCmpctVer = version
					c.Node.HighBandwidth = cmd.pl[0] == 1
				} else {
					c.counters[fmt.Sprint("SendCmpctV", version)]++
				}
				c.Mutex.Unlock()
			} else {
				common.CountSafe("SendCmpctErr")
				if len(cmd.pl) != 5 {
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

		case "getmp":
			if c.X.Authorized {
				c.ProcessGetMP(cmd.pl)
			}

		case "auth":
			c.AuthRvcd(cmd.pl)
			if c.X.AuthAckGot {
				c.GetMPNow()
			}

		case "authack":
			c.X.AuthAckGot = true
			c.GetMPNow()

		case "getmpdone":
			c.GetMPDone(cmd.pl)

		case "filterload", "filteradd", "filterclear", "merkleblock":
			c.DoS("SPV")

		default:
		}
	}

	c.GetMPDone(nil)

	c.Conn.SetWriteDeadline(time.Now()) // this should cause c.Conn.Write() to terminate
	c.writing_thread_done.Wait()

	c.Mutex.Lock()
	MutexRcv.Lock()
	for k, _ := range c.GetBlockInProgress {
		if rec, ok := BlocksToGet[k]; ok {
			rec.InProgress--
		} else {
			//println("ERROR! Block", bip.hash.String(), "in progress, but not in BlocksToGet")
		}
	}
	MutexRcv.Unlock()

	ban := c.banit
	c.Mutex.Unlock()

	if c.PeerAddr.Friend || c.X.Authorized {
		common.CountSafe(fmt.Sprint("FDisconnect-", ban))
	} else {
		if ban {
			c.PeerAddr.Ban()
			common.CountSafe("PeersBanned")
		} else if c.X.Incomming && !c.MutexGetBool(&c.X.IsSpecial) {
			var rd *RecentlyDisconenctedType
			HammeringMutex.Lock()
			rd = RecentlyDisconencted[c.PeerAddr.NetAddr.Ip4]
			if rd == nil {
				rd = &RecentlyDisconenctedType{Time:time.Now(), Count:1}
			}
			rd.Why = c.why_disconnected
			RecentlyDisconencted[c.PeerAddr.NetAddr.Ip4] = rd
			HammeringMutex.Unlock()
		}
	}
	c.Conn.Close()
}
