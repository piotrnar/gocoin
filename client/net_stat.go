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
		fmt.Printf(" %21s %5dms %7d : %-16s %7d : %-16s", v.PeerAddr.Ip(),
			v.GetAveragePing(), v.LastBtsRcvd, v.LastCmdRcvd, v.LastBtsSent, v.LastCmdSent)
		if (v.BytesReceived|v.BytesSent)!=0 {
			fmt.Printf("%9s %9s", bts(v.BytesReceived), bts(v.BytesSent))
		}
		fmt.Print("  ", v.node.agent)
		if v.send.buf !=nil {
			fmt.Print("  ", v.send.sofar, "/", len(v.send.buf))
		}
		fmt.Println()
	}

	if ExternalAddrLen()>0 {
		fmt.Print("External addresses:")
		ExternalIpMutex.Lock()
		for ip, cnt := range ExternalIp4 {
			fmt.Printf(" %d.%d.%d.%d(%d)", byte(ip>>24), byte(ip>>16), byte(ip>>8), byte(ip), cnt)
		}
		ExternalIpMutex.Unlock()
		fmt.Println()
	} else {
		fmt.Println("No known external address")
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


func node_stat(v *oneConnection) (s string) {
	s += fmt.Sprintf("Connection ID %d:\n", v.ConnID)
	if v.Incomming {
		s += fmt.Sprintln("Comming from", v.PeerAddr.Ip())
	} else {
		s += fmt.Sprintln("Going to", v.PeerAddr.Ip())
	}
	if !v.ConnectedAt.IsZero() {
		s += fmt.Sprintln("Connected at", v.ConnectedAt.Format("2006-01-02 15:04:05"))
		if v.node.version!=0 {
			s += fmt.Sprintln("Node Version:", v.node.version)
			s += fmt.Sprintln("User Agent:", v.node.agent)
			s += fmt.Sprintln("Chain Height:", v.node.height)
		}
		s += fmt.Sprintln("Last data got:", time.Now().Sub(v.LastDataGot).String())
		s += fmt.Sprintln("Last data sent:", time.Now().Sub(v.send.lastSent).String())
		s += fmt.Sprintln("Last command received:", v.LastCmdRcvd, " ", v.LastBtsRcvd, "bytes")
		s += fmt.Sprintln("Last command sent:", v.LastCmdSent, " ", v.LastBtsSent, "bytes")
		s += fmt.Sprintln("Bytes received:", v.BytesReceived)
		s += fmt.Sprintln("Bytes sent:", v.BytesSent)
		s += fmt.Sprintln("Next getbocks sending in", v.NextBlocksAsk.Sub(time.Now()).String())
		if v.LastBlocksFrom != nil {
			s += fmt.Sprintln(" Last block asked:", v.LastBlocksFrom.Height, v.LastBlocksFrom.BlockHash.String())
		}
		s += fmt.Sprintln("Ticks:", v.TicksCnt, " Loops:", v.LoopCnt)
		if v.send.buf != nil {
			s += fmt.Sprintln("Bytes to send:", len(v.send.buf), "-", v.send.sofar)
		}
		if len(v.PendingInvs)>0 {
			s += fmt.Sprintln("Invs to send:", len(v.PendingInvs))
		}

		s += fmt.Sprintln("GetBlockInProgress:", len(v.GetBlockInProgress))

		// Display ping stats
		s += fmt.Sprint("Ping history:")
		idx := v.PingHistoryIdx
		for _ = range(v.PingHistory) {
			s += fmt.Sprint(" ", v.PingHistory[idx])
			idx = (idx+1)%PingHistoryLength
		}
		s += fmt.Sprintln(" ->", v.GetAveragePing(), "ms")
	} else {
		s += fmt.Sprintln("Not yet connected")
	}
	return
}


func node_info(par string) {
	v := look2conn(par)
	if v == nil {
		fmt.Println("There is no such an active connection")
	} else {
		fmt.Print(node_stat(v))
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
