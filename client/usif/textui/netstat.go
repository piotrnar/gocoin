package textui

import (
	"fmt"
	"sort"
	"time"
	"github.com/piotrnar/gocoin/client/network"
	"github.com/piotrnar/gocoin/client/common"
	"github.com/piotrnar/gocoin/lib/others/peersdb"
)


func net_drop(par string) {
	network.DropPeer(par)
}


func node_info(par string) {
	v := network.Look4conn(par)
	if v == nil {
		fmt.Println("There is no such an active connection")
	} else {
		fmt.Print(v.Stats())
	}
}


func net_conn(par string) {
	ad, er := peersdb.NewPeerFromString(par, false)
	if er != nil {
		fmt.Println(par, er.Error())
		return
	}
	fmt.Println("Conencting to", ad.Ip())
	network.DoNetwork(ad)
}


func net_stats(par string) {
	if par=="bw" {
		common.PrintStats()
		return
	} else if par!="" {
		node_info(par)
		return
	}

	network.Mutex_net.Lock()
	fmt.Printf("%d active net connections, %d outgoing\n", len(network.OpenCons), network.OutConsActive)
	srt := make(network.SortedKeys, len(network.OpenCons))
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

		if v.Incoming {
			fmt.Print("<- ")
		} else {
			fmt.Print(" ->")
		}
		fmt.Printf(" %21s %5dms %7d : %-16s %7d : %-16s", v.PeerAddr.Ip(),
			v.GetAveragePing(), v.LastBtsRcvd, v.LastCmdRcvd, v.LastBtsSent, v.LastCmdSent)
		if (v.BytesReceived|v.BytesSent)!=0 {
			fmt.Printf("%9s %9s", common.BytesToString(v.BytesReceived), common.BytesToString(v.BytesSent))
		}
		fmt.Print("  ", v.Node.Agent)
		if v.Send.Buf !=nil {
			fmt.Print("  ", len(v.Send.Buf))
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

	fmt.Print("RecentlyDisconencted:")
	network.HammeringMutex.Lock()
	for ip, ti := range network.RecentlyDisconencted {
		fmt.Printf(" %d.%d.%d.%d-%s", ip[0], ip[1], ip[2], ip[3], time.Now().Sub(ti).String())
	}
	network.HammeringMutex.Unlock()
	fmt.Println()

	common.PrintStats()
}


func init() {
	newUi("net n", false, net_stats, "Show network statistics. Specify ID to see its details.")
	newUi("drop", false, net_drop, "Disconenct from node with a given IP")
	newUi("conn", false, net_conn, "Connect to the given node (specify IP and optionally a port)")
}
