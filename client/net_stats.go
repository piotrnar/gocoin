package main

import (
	"fmt"
	"time"
	"sort"
	"strconv"
)


type sortedkeys [] struct {
	key uint64
	ConnID uint32
}

func (sk sortedkeys) Len() int {
	return len(sk)
}

func (sk sortedkeys) Less(a, b int) bool {
	return sk[a].ConnID<sk[b].ConnID
}

func (sk sortedkeys) Swap(a, b int) {
	sk[a], sk[b] = sk[b], sk[a]
}


func look2conn(par string) (c *oneConnection) {
	conid, e := strconv.ParseUint(par, 10, 32)
	if e != nil {
		println(e.Error())
		return
	}
	mutex.Lock()
	for _, v := range openCons {
		if uint32(conid)==v.ConnID {
			c = v
			break
		}
	}
	mutex.Unlock()
	return
}


func bts(val uint64) string {
	if val < 1e6 {
		return fmt.Sprintf("%.1f KB", float64(val)/1e3)
	} else if val < 1e9 {
		return fmt.Sprintf("%.2f MB", float64(val)/1e6)
	}
	return fmt.Sprintf("%.2f GB", float64(val)/1e9)
}


func net_stats(par string) {
	if par=="bw" {
		bw_stats()
		return
	} else if par!="" {
		node_info(par)
		return
	}

	mutex.Lock()
	fmt.Printf("%d active net connections, %d outgoing\n", len(openCons), OutConsActive)
	srt := make(sortedkeys, len(openCons))
	cnt := 0
	for k, v := range openCons {
		srt[cnt].key = k
		srt[cnt].ConnID = v.ConnID
		cnt++
	}
	sort.Sort(srt)
	for idx := range srt {
		v := openCons[srt[idx].key]
		fmt.Printf("%8d) ", v.ConnID)

		if v.Incomming {
			fmt.Print("<- ")
		} else {
			fmt.Print(" ->")
		}
		fmt.Printf(" %21s %7d : %-16s %7d : %-16s", v.PeerAddr.Ip(),
			v.LastBtsRcvd, v.LastCmdRcvd, v.LastBtsSent, v.LastCmdSent)
		if (v.BytesReceived|v.BytesSent)!=0 {
			fmt.Printf("%9s %9s", bts(v.BytesReceived), bts(v.BytesSent))
		}
		fmt.Print("  ", v.node.agent)
		if v.send.buf !=nil {
			fmt.Print("  ", v.send.sofar, "/", len(v.send.buf))
		}
		fmt.Println()
	}
	if *server && MyExternalAddr!=nil {
		fmt.Println("TCP server listening at external address", MyExternalAddr.String())
	}
	mutex.Unlock()
	bw_stats()
}


func net_drop(par string) {
	c := look2conn(par)
	if c!=nil {
		c.Broken = true
		fmt.Println("The connection with", c.PeerAddr.Ip(), "is being dropped")
	} else {
		fmt.Println("There is no such an active connection")
	}
}


func node_info(par string) {
	v := look2conn(par)
	if v == nil {
		fmt.Println("There is no such an active connection")
	}

	fmt.Printf("Connection ID %d:\n", v.ConnID)
	if v.Incomming {
		fmt.Println("Comming from", v.PeerAddr.Ip())
	} else {
		fmt.Println("Going to", v.PeerAddr.Ip())
	}
	if !v.ConnectedAt.IsZero() {
		fmt.Println(" Connected at", v.ConnectedAt.Format("2006-01-02 15:04:05"))
		if v.node.version!=0 {
			fmt.Println(" Node Version:", v.node.version)
			fmt.Println(" User Agent:", v.node.agent)
			fmt.Println(" Chain Height:", v.node.height)
		}
		fmt.Println(" Last data got/sent:", time.Now().Sub(v.LastDataGot).String())
		fmt.Println(" Last command received:", v.LastCmdRcvd, " ", v.LastBtsRcvd, "bytes")
		fmt.Println(" Last command sent:", v.LastCmdSent, " ", v.LastBtsSent, "bytes")
		fmt.Println(" Bytes received:", v.BytesReceived)
		fmt.Println(" Bytes sent:", v.BytesSent)
		fmt.Println(" Next getbocks sending in", v.NextBlocksAsk.Sub(time.Now()).String())
		if v.LastBlocksFrom != nil {
			fmt.Println(" Last block asked:", v.LastBlocksFrom.Height, v.LastBlocksFrom.BlockHash.String())
		}
		fmt.Println(" Ticks:", v.TicksCnt, " Loops:", v.LoopCnt)
		if v.send.buf != nil {
			fmt.Println(" Bytes to send:", len(v.send.buf), "-", v.send.sofar)
		}
		if len(v.PendingInvs)>0 {
			fmt.Println(" Invs to send:", len(v.PendingInvs))
		}
	} else {
		fmt.Println("Not yet connected")
	}
}


func net_conn(par string) {
	ad, er := NewIncommingPeer(par)
	if er != nil {
		fmt.Println(par, er.Error())
		return
	}
	fmt.Println("Conencting to", ad.Ip())
	do_network(ad)
}


func init() {
	newUi("net n", false, net_stats, "Show network statistics. Specify ID to see its details.")
	newUi("drop", false, net_drop, "Disconenct from node with a given IP")
	newUi("conn", false, net_conn, "Connect to the given node (specify IP and optionally a port)")
}
