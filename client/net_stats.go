package main

import (
	"fmt"
	"time"
	"sort"
	"strconv"
)


type sortedkeys [] struct {
	key uint64
	id uint32
}

func (sk sortedkeys) Len() int {
	return len(sk)
}

func (sk sortedkeys) Less(a, b int) bool {
	return sk[a].id<sk[b].id
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
		if uint32(conid)==v.id {
			c = v
			break
		}
	}
	mutex.Unlock()
	return
}


func bts(val uint64) {
	if val < 1e5*1024 {
		fmt.Printf("%9.1f k ", float64(val)/1024)
	} else {
		fmt.Printf("%9.1f MB", float64(val)/(1024*1024))
	}
}


func net_stats(par string) {
	mutex.Lock()
	fmt.Printf("%d active net connections, %d outgoing\n", len(openCons), OutConsActive)
	srt := make(sortedkeys, len(openCons))
	cnt := 0
	for k, v := range openCons {
		srt[cnt].key = k
		srt[cnt].id = v.id
		cnt++
	}
	sort.Sort(srt)
	fmt.Println()
	for idx := range srt {
		v := openCons[srt[idx].key]
		fmt.Printf("%8d) ", v.id)

		if v.Incomming {
			fmt.Print("<- ")
		} else {
			fmt.Print(" ->")
		}
		fmt.Printf(" %21s  %-16s", v.addr.Ip(), v.last_cmd)
		if (v.BytesReceived|v.BytesSent)!=0 {
			bts(v.BytesReceived)
			bts(v.BytesSent)
			if v.sendbuf !=nil {
				fmt.Print("  ", v.sentsofar, "/", len(v.sendbuf))
			}
		}
		fmt.Println()
	}
	fmt.Printf("InvsSent:%d,  BlockSent:%d,  Timeouts:%d\n", 
		InvsSent, BlockSent, ConnTimeoutCnt)
	if *server && MyExternalAddr!=nil {
		fmt.Println("TCP server listening at external address", MyExternalAddr.String())
	}
	mutex.Unlock()
}


func net_drop(par string) {
	c := look2conn(par)
	if c!=nil {
		c.broken = true
		fmt.Println("The connection with", c.addr.Ip(), "is being dropped")
	} else {
		fmt.Println("There is no such an active connection")
	}
}


func node_info(par string) {
	v := look2conn(par)
	if v == nil {
		fmt.Println("There is no such an active connection")
	}

	fmt.Printf("Connection ID %d:\n", v.id)
	if v.Incomming {
		fmt.Println("Comming from", v.addr.Ip())
	} else {
		fmt.Println("Going to", v.addr.Ip())
	}
	if !v.ConnectedAt.IsZero() {
		fmt.Println(" Connected at", v.ConnectedAt.Format("2006-01-02 15:04:05"))
		fmt.Println(" Last data got/sent:", time.Now().Sub(v.LastDataGot).String())
		fmt.Println(" Last command:", v.last_cmd)
		fmt.Println(" Bytes received:", v.BytesReceived)
		fmt.Println(" Bytes sent:", v.BytesSent)
		fmt.Println(" Next addr sending in: ", v.NextAddrSent.Sub(time.Now()).String())
		if v.node.version!=0 {
			fmt.Println(" Node Version:", v.node.version)
			fmt.Println(" User Agent:", v.node.agent)
			fmt.Println(" Chain Height:", v.node.height)
		}
		fmt.Println(" Ticks:", v.ticks)
		fmt.Println(" Loops:", v.loops)
		if v.sendbuf != nil {
			fmt.Println(" Bytes to send:", len(v.sendbuf), "-", v.sentsofar)
		}
		if len(v.invs2send)>0 {
			fmt.Println(" Invs to send:", len(v.invs2send))
		}
	} else {
		fmt.Println("Not yet connected")
	}
}


func init() {
	newUi("net", false, net_stats, "Show network statistics")
	newUi("node n", false, node_info, "Show information about the specific node")
	newUi("drop", false, net_drop, "Disconenct from node with a given IP")
}

