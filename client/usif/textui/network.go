package textui

import (
	"fmt"
	"sort"
	"time"
	"strconv"
	"github.com/piotrnar/gocoin/client/network"
	"github.com/piotrnar/gocoin/client/common"
	"github.com/piotrnar/gocoin/lib/others/peersdb"
)


type SortedKeys [] struct {
	Key uint64
	ConnID uint32
}

func (sk SortedKeys) Len() int {
	return len(sk)
}

func (sk SortedKeys) Less(a, b int) bool {
	return sk[a].ConnID<sk[b].ConnID
}

func (sk SortedKeys) Swap(a, b int) {
	sk[a], sk[b] = sk[b], sk[a]
}


func net_drop(par string) {
	conid, e := strconv.ParseUint(par, 10, 32)
	if e != nil {
		println(e.Error())
		return
	}
	network.DropPeer(uint32(conid))
}


func node_info(par string) {
	conid, e := strconv.ParseUint(par, 10, 32)
	if e != nil {
		return
	}

	var r *network.ConnInfo

	network.Mutex_net.Lock()

	for _, v := range network.OpenCons {
		if uint32(conid)==v.ConnID {
			r = new(network.ConnInfo)
			v.GetStats(r)
			break
		}
	}
	network.Mutex_net.Unlock()

	if r == nil {
		return
	}

	fmt.Printf("Connection ID %d:\n", r.ID)
	if r.Incomming {
		fmt.Println("Coming from", r.PeerIp)
	} else {
		fmt.Println("Going to", r.PeerIp)
	}
	if !r.ConnectedAt.IsZero() {
		fmt.Println("Connected at", r.ConnectedAt.Format("2006-01-02 15:04:05"))
		if r.Version!=0 {
			fmt.Println("Node Version:", r.Version, "/ Services:", fmt.Sprintf("0x%x", r.Services))
			fmt.Println("User Agent:", r.Agent)
			fmt.Println("Chain Height:", r.Height)
			fmt.Printf("Reported IP: %d.%d.%d.%d\n", byte(r.ReportedIp4>>24), byte(r.ReportedIp4>>16),
				byte(r.ReportedIp4>>8), byte(r.ReportedIp4))
			fmt.Println("SendHeaders:", r.SendHeaders)
		}
		fmt.Println("Invs Done:", r.InvsDone)
		fmt.Println("Last data got:", time.Now().Sub(r.LastDataGot).String())
		fmt.Println("Last data sent:", time.Now().Sub(r.LastSent).String())
		fmt.Println("Last command received:", r.LastCmdRcvd, " ", r.LastBtsRcvd, "bytes")
		fmt.Println("Last command sent:", r.LastCmdSent, " ", r.LastBtsSent, "bytes")
		fmt.Print("Invs  Recieved:", r.InvsRecieved, "  Pending:", r.InvsToSend, "\n")
		fmt.Print("Bytes to send:", r.BytesToSend, " (", r.MaxSentBufSize, " max)\n")
		fmt.Print("BlockInProgress:", r.BlocksInProgress, "  GetHeadersInProgress:", r.GetHeadersInProgress, "\n")
		fmt.Println("GetBlocksDataNow:", r.GetBlocksDataNow)
		fmt.Println("AllHeadersReceived:", r.AllHeadersReceived)
		fmt.Println("Total Received:", r.BytesReceived, " /  Sent:", r.BytesSent)
		for k, v := range r.Counters {
			fmt.Println(k, ":", v)
		}
	} else {
		fmt.Println("Not yet connected")
	}
}


func net_conn(par string) {
	ad, er := peersdb.NewAddrFromString(par, false)
	if er != nil {
		fmt.Println(par, er.Error())
		return
	}
	fmt.Println("Connecting to", ad.Ip())
	ad.Manual = true
	network.DoNetwork(ad)
}


func net_stats(par string) {
	if par=="bw" {
		common.PrintBWStats()
		return
	} else if par!="" {
		node_info(par)
		return
	}

	network.Mutex_net.Lock()
	fmt.Printf("%d active net connections, %d outgoing\n", len(network.OpenCons), network.OutConsActive)
	srt := make(SortedKeys, len(network.OpenCons))
	cnt := 0
	for k, v := range network.OpenCons {
		srt[cnt].Key = k
		srt[cnt].ConnID = v.ConnID
		cnt++
	}
	sort.Sort(srt)
	for idx := range srt {
		v := network.OpenCons[srt[idx].Key]
		v.Mutex.Lock()
		fmt.Printf("%8d) ", v.ConnID)

		if v.X.Incomming {
			fmt.Print("<- ")
		} else {
			fmt.Print(" ->")
		}
		fmt.Printf(" %21s %5dms", v.PeerAddr.Ip(), v.GetAveragePing())
		//fmt.Printf(" %7d : %-16s %7d : %-16s", v.X.LastBtsRcvd, v.X.LastCmdRcvd, v.X.LastBtsSent, v.X.LastCmdSent)
		fmt.Printf("%9s %9s", common.BytesToString(v.X.Counters["BytesReceived"]), common.BytesToString(v.X.Counters["BytesSent"]))
		fmt.Print("  ", v.Node.Agent)

		if b2s:=v.BytesToSent(); b2s>0 {
			fmt.Print("  ", b2s)
		}
		v.Mutex.Unlock()
		fmt.Println()
	}

	if network.ExternalAddrLen()>0 {
		fmt.Print("External addresses:")
		network.ExternalIpMutex.Lock()
		for ip, cnt := range network.ExternalIp4 {
			fmt.Printf(" %d.%d.%d.%d(%d)", byte(ip>>24), byte(ip>>16), byte(ip>>8), byte(ip), cnt)
		}
		network.ExternalIpMutex.Unlock()
		fmt.Println()
	} else {
		fmt.Println("No known external address")
	}

	network.Mutex_net.Unlock()

	fmt.Println("GetMPInProgress:", len(network.GetMPInProgressTicket) != 0)

	common.PrintBWStats()
}


func net_rd(par string) {
	network.HammeringMutex.Lock()
	srt := make([]string, len(network.RecentlyDisconencted))
	var idx int
	for ip, rd := range network.RecentlyDisconencted {
		srt[idx] = fmt.Sprintf("%31d %16s %3d %16s - %s", rd.Time.UnixNano(),
			fmt.Sprintf("%d.%d.%d.%d", ip[0], ip[1], ip[2], ip[3]), rd.Count,
			time.Now().Sub(rd.Time).String(), rd.Why)
		idx++
	}
	sort.Strings(srt)
	network.HammeringMutex.Unlock()
	fmt.Println("Recently disconencted incoming connections:")
	for _, s := range srt {
		fmt.Println(s[32:])
	}
	fmt.Println("Baned if", network.HammeringMaxAllowedCount+1, "conections (within", network.HammeringMinReconnect,
		"...", network.HammeringMinReconnect+network.HammeringExpirePeriod, "in between)")
}


func init() {
	newUi("net n", false, net_stats, "Show network statistics. Specify ID to see its details.")
	newUi("drop", false, net_drop, "Disconenct from node with a given IP")
	newUi("conn", false, net_conn, "Connect to the given node (specify IP and optionally a port)")
	newUi("rd", false, net_rd, "Show recently disconnected incoming connections")
}
