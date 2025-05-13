package network

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"math/rand"
	"net"
	"os"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"github.com/piotrnar/gocoin/client/common"
	"github.com/piotrnar/gocoin/client/peersdb"
	"github.com/piotrnar/gocoin/client/txpool"
	"github.com/piotrnar/gocoin/lib/btc"
	"github.com/piotrnar/gocoin/lib/others/sys"
)

var (
	TCPServerStarted   bool
	next_drop_peer     time.Time
	next_clean_hammers time.Time

	NextConnectFriends time.Time = time.Now()
	AuthPubkeys        [][]byte
	SpecialAgents      []string
	SpecialIPs         [][4]byte
	FriendsAccess      sync.Mutex

	GetMPInProgressConnID sys.SyncInt
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
			c.cntInc("BlockNotFound")
		} else if now != nil && now.After(v.start.Add(5*time.Minute)) {
			common.CountSafe("BlockInprogTimeout")
			c.cntInc("BlockTimeout")
		} else {
			continue
		}
		c.X.BlocksExpired++
		delete(c.GetBlockInProgress, k)
		if bip, ok := BlocksToGet[k]; ok {
			bip.InProgress--
			bip.FailCount++
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
		common.CountSafe(disconnect)
		if c.X.IsSpecial {
			common.CountSafe(disconnect + "Spec")
			c.cntInc(disconnect)
		} else if !common.Get(&common.BlockChainSynchronized) {
			c.Disconnect(true, disconnect)
		} else {
			common.CountSafe(disconnect + "Sync")
		}
	}
}

// Call this once a minute
func (c *OneConnection) Maintanence(now time.Time) {
	// Expire GetBlockInProgress after five minutes, if they are not in BlocksToGet
	c.ExpireHeadersAndGetData(&now, 0)

	// Expire BlocksReceived counter
	c.Mutex.Lock()
	if len(c.blocksreceived) > 0 {
		expire_before := now.Add(-common.Get(&common.BlockExpireEvery))
		var remove_blocks_cnt uint64
		for _, br := range c.blocksreceived {
			if br.After(expire_before) {
				break
			}
			remove_blocks_cnt++
		}
		if remove_blocks_cnt > 0 {
			c.blocksreceived = c.blocksreceived[remove_blocks_cnt:]
			common.CountSafeAdd("BlksRcvdExpired", remove_blocks_cnt)
		}
	}
	c.Mutex.Unlock()
}

func (c *OneConnection) Tick(now time.Time) {
	c.Mutex.Lock()
	c.X.Ticks++
	if common.NoCounters.Get() && len(c.counters) > 0 {
		c.counters = make(map[string]uint64) // reset all the counters
	}
	c.Mutex.Unlock()

	if !c.X.VersionReceived {
		// Wait only certain amount of time for the version message
		if c.X.ConnectedAt.Add(VersionMsgTimeout).Before(now) {
			c.Disconnect(true, "VersionTimeout")
			common.CountSafe("NetVersionTout")
			return
		}
		// Until we receive version message, do nothing more.
		return
	}

	var new_sec bool
	if tck := now.Unix(); c.lastSec != tck {
		// these opertions will only be done once a second
		new_sec = true // keep this for later (to try a ping)
		c.lastSec = tck

		// If we have not received any data for some time, disconnect
		if now.Sub(c.X.LastDataGot) > NoDataTimeout {
			c.Disconnect(true, "NoDataTimeout")
			common.CountSafe("NetNoDataTout")
			return
		}

		if len(c.GetMP) > 0 && common.Get(&common.BlockChainSynchronized) {
			// See if to send "getmp" command
			select {
			case txpool.GetMPInProgressTicket <- true:
				// ticket received - check for the request...
				GetMPInProgressConnID.Store(int(c.ConnID))
				if c.SendGetMP() != nil {
					// SendGetMP() failed - clear the global flag/channel
					<-txpool.GetMPInProgressTicket
					<-c.GetMP
				}
			default:
				// failed to get the ticket - just do nothing
			}
		}

		// Tick the recent transactions counter
		if now.After(c.txsNxt) {
			c.Mutex.Lock()
			if c.txsCurIdx == len(c.txsCha)-1 {
				c.txsCurIdx = 0
			} else {
				c.txsCurIdx++
			}
			c.X.TxsReceived -= int(c.txsCha[c.txsCurIdx])
			c.txsCha[c.txsCurIdx] = 0
			c.txsNxt = c.txsNxt.Add(TxsCounterTick)
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
		if !c.X.OurGetAddrDone && peersdb.PeerDB.Count() < peersdb.MinPeersInDB {
			common.CountSafe("AddrsWanted")
			c.SendRawMsg("getaddr", nil, false)
			c.X.OurGetAddrDone = true
		}
	}

	c.Mutex.Lock()
	if c.HasNetworkService() && !c.X.GetHeadersInProgress && !c.X.AllHeadersReceived && len(c.GetBlockInProgress) == 0 {
		c.Mutex.Unlock()
		c.sendGetHeaders()
		return // new headers requested
	}

	if c.X.AllHeadersReceived {
		if !c.X.GetBlocksDataNow && now.After(c.nextGetData) {
			c.X.GetBlocksDataNow = true
		}
		if c.X.GetBlocksDataNow && len(c.GetBlockInProgress) <= c.keepBlocksOver {
			c.X.GetBlocksDataNow = false
			c.Mutex.Unlock()
			c.GetBlockData()
			return // block data requested
		}
	}
	c.Mutex.Unlock()

	if new_sec { // nothing requested - free to ping..
		c.TryPing(now)
	}
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
	conn.addToList()
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
				} else {
					conn.dead = true
				}
			case <-time.After(10 * time.Millisecond):
				if !conn.IsBroken() {
					continue
				}
			}
			break
		}

		Mutex_net.Lock()
		conn.delFromList()
		OutConsActive--
		Mutex_net.Unlock()
		if conn.dead {
			ad.Dead()
		} else {
			ad.Save()
		}
	}()
}

// TCP server
func tcp_server() {
	var ad net.TCPAddr
	ad.IP = net.ParseIP(common.CFG.Net.BindToIF)
	if ad.IP == nil {
		println("Check config value of Net.BindToIF - binding to any...")
		ad.IP = net.IPv4(0, 0, 0, 0)
	}
	ad.Port = int(common.ConfiguredTcpPort())

	lis, e := net.ListenTCP("tcp4", &ad)
	if e != nil {
		println("ListenTCP", e.Error())
		return
	}
	defer lis.Close()

	//fmt.Println("TCP server started at", ad.String())

	for common.IsListenTCP() {
		//common.CountSafe("NetServerLoops")
		Mutex_net.Lock()
		ica := InConsActive
		Mutex_net.Unlock()
		if ica < common.Get(&common.CFG.Net.MaxInCons) {
			lis.SetDeadline(time.Now().Add(100 * time.Millisecond))
			tc, e := lis.AcceptTCP()
			if e == nil && common.IsListenTCP() {
				var terminate bool

				// set port to default, for incomming connections
				ad, e := peersdb.NewIncommingConnection(tc.RemoteAddr().String(), true)
				if e == nil {
					//println("incomming connection from", ad.Ip(), tc.RemoteAddr().String())
					// Hammering protection
					HammeringMutex.Lock()
					if rd := RecentlyDisconencted[ad.NetAddr.Ip4]; rd != nil {
						rd.Count++
						terminate = rd.Count > HammeringMaxAllowedCount
					}
					HammeringMutex.Unlock()

					if terminate {
						common.CountSafe("BanHammerIn")
						ad.Ban("HammerIn")
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
							conn.addToList()
							InConsActive++
							Mutex_net.Unlock()
							go func() {
								conn.Run()
								Mutex_net.Lock()
								conn.delFromList()
								InConsActive--
								Mutex_net.Unlock()
							}()
						}
					}
				} else {
					common.CountSafe("InConnDenied")
					terminate = true
				}

				// had any error occured - close the TCP connection
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
			c.Disconnect(false, "CloseAllIn")
		}
	}
	TCPServerStarted = false
	Mutex_net.Unlock()
	//fmt.Println("TCP server stopped")
}

var friends_pubkey_cache map[string][]byte

func ConnectFriends() {
	common.CountSafe("ConnectFriends")

	f, _ := os.Open(common.GocoinHomeDir + "friends.txt")
	if f == nil {
		return
	}
	defer f.Close()

	var auth_pubkeys [][]byte
	var special_agents []string
	var special_ips [][4]byte
	var addrs_to_connect []*peersdb.PeerAddr
	friend_ids := make(map[uint64]bool)

	new_pubkey_cache := make(map[string][]byte)
	rd := bufio.NewReader(f)
	if rd != nil {
		for {
			ln, _, er := rd.ReadLine()
			if er != nil {
				break
			}
			lns := strings.Trim(string(ln), " \r\n\t")
			if len(lns) == 0 || lns[0] == '#' {
				continue
			}
			ls := strings.SplitN(lns, " ", 2)
			if len(ls[0]) > 1 {
				var done bool
				switch ls[0][0] {
				case '@':
					var pk []byte
					pks := ls[0][1:]
					if friends_pubkey_cache != nil {
						pk = friends_pubkey_cache[pks]
						//println(" - from cache:", len(pk))
					}
					if pk == nil {
						pk = btc.Decodeb58(pks)
					}
					if len(pk) == 33 {
						new_pubkey_cache[pks] = pk
						auth_pubkeys = append(auth_pubkeys, pk)
						//println("Using Auth Key:", hex.EncodeToString(pk))
					} else {
						println(pks, "is not a valid Auth Key. Check your friends.txt file")
					}

				case '+':
					if ad, _ := peersdb.NewAddrFromString(ls[0][1:], false); ad != nil {
						special_ips = append(special_ips, ad.Ip4)
					}

				case '*':
					special_agents = append(special_agents, ls[0][1:])

				}
				if done {
					continue
				}
			}
			if peersdb.ConnectOnly != "" {
				// Do not connect to friends in single connection mode
				continue
			}
			ad, _ := peersdb.NewAddrFromString(ls[0], false)
			if ad != nil {
				//println(" Trying to connect", ad.Ip())
				addrs_to_connect = append(addrs_to_connect, ad)
				continue
			}
		}
	}
	if len(new_pubkey_cache) > 0 {
		friends_pubkey_cache = new_pubkey_cache
	} else {
		friends_pubkey_cache = nil
	}
	FriendsAccess.Lock()
	AuthPubkeys = auth_pubkeys
	SpecialAgents = special_agents
	SpecialIPs = special_ips
	FriendsAccess.Unlock()

	for _, ad := range addrs_to_connect {
		Mutex_net.Lock()
		curr := OpenCons[ad.UniqID()]
		Mutex_net.Unlock()
		if curr == nil {
			ad.Friend = true
			DoNetwork(ad)
		} else {
			curr.Mutex.Lock()
			curr.PeerAddr.Friend = true
			curr.X.IsSpecial = true
			curr.Mutex.Unlock()
		}
		friend_ids[ad.UniqID()] = true
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
		v.Mutex.Lock()
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

	if common.Get(&common.CFG.DropPeers.DropEachMinutes) != 0 &&
		common.Get(&common.CFG.DropPeers.ImmunityMinutes) != 0 {
		if next_drop_peer.IsZero() {
			next_drop_peer = now.Add(common.Get(&common.DropSlowestEvery))
		} else if now.After(next_drop_peer) {
			if drop_worst_peer() {
				next_drop_peer = now.Add(common.Get(&common.DropSlowestEvery))
			} else {
				// If no peer dropped this time, try again sooner
				next_drop_peer = now.Add(common.Get(&common.DropSlowestEvery) >> 2)
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

	if conn_cnt < common.Get(&common.CFG.Net.MaxOutCons) {
		// First we will choose up to 128 peers that we have seen alive - do not sort them
		adrs := peersdb.GetRecentPeers(128, false, func(ad *peersdb.PeerAddr) bool {
			return ad.Banned != 0 || !ad.SeenAlive || (ad.Services&btc.SERVICE_SEGWIT) == 0 || ConnectionActive(ad)
		})
		// now fetch another 32 never tried peers (this time sorted)
		new_cnt := int(32)
		if len(adrs) > new_cnt {
			new_cnt = len(adrs)
		}
		adrs2 := peersdb.GetRecentPeers(uint(new_cnt), true, func(ad *peersdb.PeerAddr) bool {
			return ad.Banned != 0 || ad.SeenAlive || (ad.Services&btc.SERVICE_SEGWIT) == 0 // ignore those that have been seen alive
		})
		adrs = append(adrs, adrs2...)
		// Now we should have 128 peers known to be alive and 32 never tried ones
		// ... giving us 20% chance of selecting a never tried one.
		if len(adrs) != 0 {
			ad := adrs[rand.Int31n(int32(len(adrs)))]
			//print("chosen ", ad.String(), "\n> ")
			DoNetwork(ad)
		}
	}
	txpool.Tick()
}

func (c *OneConnection) SendFeeFilter() {
	var pl [8]byte
	binary.LittleEndian.PutUint64(pl[:], c.X.LastMinFeePerKByte)
	c.SendRawMsg("feefilter", pl[:], false)
}

// GetMPDone should be called upon receiving "getmpdone" message or when the peer disconnects.
func (c *OneConnection) GetMPDone(pl []byte) {
	if len(c.GetMP) == 0 {
		return
	}
	if len(txpool.GetMPInProgressTicket) == 0 {
		// This will happen when our chain is not yet synchronized and we are disconnecting a peer which have sent "authack"
		return
	}

	if GetMPInProgressConnID.Get() != int(c.ConnID) {
		if len(pl) < 1 {
			<-c.GetMP
		} else {
			println("PEER", c.ConnID, "MISBEHAVE: Sent getmpdone but ticket", GetMPInProgressConnID.Get(), "held elsewere")
		}
		return
	}

	// the ticket is ours
	if len(pl) < 1 || pl[0] == 0 {
		<-c.GetMP
	} else if c.SendGetMP() != nil {
		<-c.GetMP
	} else {
		return
	}

	if len(txpool.GetMPInProgressTicket) == 0 {
		// TODO: remove it at some point (should not be happening)
		println("ERROR: GetMPDone() exiting without a ticket (will hang)")
	}
	<-txpool.GetMPInProgressTicket
}

// Run starts a process that handles communication with a single peer.
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

	if !c.X.Incomming {
		c.SendVersion()
	}

	c.Mutex.Lock()
	now := time.Now()
	c.X.LastDataGot = now
	c.nextMaintanence = now.Add(time.Minute)
	c.LastPingSent = now.Add(5*time.Second - common.Get(&common.PingPeerEvery)) // do first ping ~5 seconds from now

	c.txsNxt = now.Add(TxsCounterTick)

	c.Mutex.Unlock()

	next_tick := now
	next_invs := now

	c.writing_thread_done.Add(1)
	go c.writing_thread()

	for !c.IsBroken() {
		cmd, read_tried := c.FetchMessage()

		now = time.Now()
		if c.X.VersionReceived && (c.sendInvsNow.Get() || now.After(next_invs)) {
			c.SendInvs()
			next_invs = now.Add(InvsFlushPeriod)
		}

		if now.After(next_tick) {
			c.Tick(now)
			next_tick = now.Add(PeerTickPeriod)
		}

		if cmd == nil {
			if c.unfinished_getdata != nil && !c.SendingPaused() {
				common.CountSafe("GetDataRestored")
				tmp := c.unfinished_getdata.Bytes()
				//println(c.ConnID, "restoring getdata for", len(tmp)/36, "invs")
				c.unfinished_getdata = nil
				c.processGetData(bytes.NewReader(tmp))
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
				//println("VersionAgain from", c.ConnID, c.PeerAddr.Ip(), c.Node.Agent)
				c.Misbehave("VersionAgain", 1000/10)
				continue
			}
			if c.X.Incomming {
				//println(c.PeerAddr.Ip(), "sending our own version...")
				c.SendVersion()
			}
			er := c.HandleVersion(cmd.pl)
			if er != nil {
				//println("version msg error:", er.Error())
				c.DoS("Ver" + er.Error())
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

			if c.Node.Version >= 70012 && c.HasNetworkService() {
				c.SendRawMsg("sendheaders", nil, false)
				if c.Node.Version >= 70013 {
					if c.X.LastMinFeePerKByte != 0 {
						c.SendFeeFilter()
					}
					if c.Node.Version >= 70014 && common.Get(&common.CFG.TXPool.Enabled) {
						// ask for compact blocks version 2 only
						c.SendRawMsg("sendcmpct", []byte{0x01, 0x02, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}, false)
					}
				}
			}
			c.PeerAddr.Services = c.Node.Services
			c.PeerAddr.NodeAgent = c.Node.Agent
			c.PeerAddr.Alive()

			if common.IsListenTCP() {
				c.SendOwnAddr()
			}
			continue
		} else if !c.X.VersionReceived {
			//println(c.PeerAddr.Ip(), "version not received but", cmd.cmd)
			c.Misbehave("NoVer"+cmd.cmd, 1000/10)
			continue
		}

		switch cmd.cmd {
		case "inv":
			c.ProcessInv(cmd.pl)

		case "tx":
			if common.AcceptTx() {
				c.ParseTxNet(cmd)
			}

		case "addr":
			c.ParseAddr(cmd.pl)

		case "block": //block received
			c.netBlockReceived(cmd)
			c.MutexSetBool(&c.X.GetBlocksDataNow, true) // ask for more blocks during next tick

		case "getblocks":
			c.GetBlocks(cmd.pl)

		case "getdata":
			c.ProcessGetData(cmd.pl)

		case "getaddr":
			if !c.X.GetAddrDone {
				c.HandleGetaddr()
				c.X.GetAddrDone = true
			} else {
				c.Mutex.Lock()
				c.cntInc("SecondGetAddr")
				c.Mutex.Unlock()
				if c.Misbehave("SecondGetAddr", 1000/20) {
					break
				}
			}

		case "ping":
			re := make([]byte, len(cmd.pl))
			copy(re, cmd.pl)
			c.SendRawMsg("pong", re, false)

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
					c.cntInc(fmt.Sprint("SendCmpctV", version))
				}
				c.Mutex.Unlock()
			} else {
				common.CountSafe("SendCmpctErr")
				if len(cmd.pl) != 5 {
					println(c.ConnID, c.PeerAddr.Ip(), c.Node.Agent, "sendcmpct", hex.EncodeToString(cmd.pl))
				}
			}

		case "cmpctblock":
			if common.Get(&common.BlockChainSynchronized) {
				c.ProcessCmpctBlock(cmd)
			}

		case "getblocktxn":
			c.ProcessGetBlockTxn(cmd.pl)
			//println(c.ConnID, c.PeerAddr.Ip(), c.Node.Agent, "getblocktxn", hex.EncodeToString(cmd.pl))

		case "blocktxn":
			c.ProcessBlockTxn(cmd)
			//println(c.ConnID, c.PeerAddr.Ip(), c.Node.Agent, "blocktxn", hex.EncodeToString(cmd.pl))

		case "getmp":
			if c.X.Authorized {
				c.ProcessGetMP(cmd.pl)
			}

		case "xauth": // "xauth" supports encryption and makes "auth" deprecated as not secure
			c.AuthRvcd(cmd.pl)

		case "authack":
			if !cmd.trusted {
				println(c.PeerAddr.Ip(), "sent us unsigned authack")
				return
			}
			c.Mutex.Lock()
			c.X.AuthAckGot = true
			c.Mutex.Unlock()
			if len(cmd.pl) > 0 {
				// if there is payload, the first byte says if the node is synchronized
				c.X.ChainSynchronized = cmd.pl[0] != 0
			}
			if c.X.ChainSynchronized {
				c.GetMPNow() // No point in asking non-synched nodes for their mempool
			}

		case "getmpdone":
			c.GetMPDone(cmd.pl)

		case "filterload", "filteradd", "filterclear", "merkleblock":
			c.DoS("SPV")

		default:
		}
	}

	c.GetMPDone(nil) // release the ticket, if kept

	c.Conn.SetWriteDeadline(time.Now()) // this should cause c.Conn.Write() to terminate
	c.writing_thread_done.Wait()

	c.Mutex.Lock()
	MutexRcv.Lock()
	for k := range c.GetBlockInProgress {
		if rec, ok := BlocksToGet[k]; ok {
			rec.InProgress--
		}
	}
	MutexRcv.Unlock()

	ban := c.banit
	c.Mutex.Unlock()

	if c.PeerAddr.Friend || c.X.Authorized {
		common.CountSafePar("FDisconnect-", ban)
	} else {
		if ban {
			c.PeerAddr.Ban(c.ban_reason)
			common.CountSafe("PeersBanned")
		} else if c.X.Incomming && !c.MutexGetBool(&c.X.Authorized) {
			var rd *RecentlyDisconenctedType
			HammeringMutex.Lock()
			rd = RecentlyDisconencted[c.PeerAddr.NetAddr.Ip4]
			if rd == nil {
				rd = &RecentlyDisconenctedType{Time: time.Now(), Count: 1}
			}
			rd.Why = c.why_disconnected
			RecentlyDisconencted[c.PeerAddr.NetAddr.Ip4] = rd
			HammeringMutex.Unlock()
		}
	}
	c.Conn.Close()
}
