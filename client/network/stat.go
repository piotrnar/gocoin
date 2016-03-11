package network

import (
	"fmt"
	"time"
	"strconv"
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


func Look4conn(par string) (c *OneConnection) {
	conid, e := strconv.ParseUint(par, 10, 32)
	if e != nil {
		println(e.Error())
		return
	}
	Mutex_net.Lock()
	for _, v := range OpenCons {
		if uint32(conid)==v.ConnID {
			c = v
			break
		}
	}
	Mutex_net.Unlock()
	return
}


func (v *OneConnection) Stats() (s string) {
	s += fmt.Sprintf("Connection ID %d:\n", v.ConnID)
	if v.Incoming {
		s += fmt.Sprintln("Comming from", v.PeerAddr.Ip())
	} else {
		s += fmt.Sprintln("Going to", v.PeerAddr.Ip())
	}
	if !v.ConnectedAt.IsZero() {
		v.Mutex.Lock()
		s += fmt.Sprintln("Connected at", v.ConnectedAt.Format("2006-01-02 15:04:05"))
		if v.Node.Version!=0 {
			s += fmt.Sprintln("Node Version:", v.Node.Version, "/ Services:", fmt.Sprintf("0x%x", v.Node.Services))
			s += fmt.Sprintln("User Agent:", v.Node.Agent)
			s += fmt.Sprintln("Chain Height:", v.Node.Height)
			s += fmt.Sprintf("Reported IP: %d.%d.%d.%d\n", byte(v.Node.ReportedIp4>>24), byte(v.Node.ReportedIp4>>16),
				byte(v.Node.ReportedIp4>>8), byte(v.Node.ReportedIp4))
			s += fmt.Sprintln("SendHeaders:", v.Node.SendHeaders)
		}
		s += fmt.Sprintln("Last data got:", time.Now().Sub(v.LastDataGot).String())
		s += fmt.Sprintln("Last data sent:", time.Now().Sub(v.Send.LastSent).String())
		s += fmt.Sprintln("Last command received:", v.LastCmdRcvd, " ", v.LastBtsRcvd, "bytes")
		s += fmt.Sprintln("Last command sent:", v.LastCmdSent, " ", v.LastBtsSent, "bytes")
		s += fmt.Sprintln("Ticks:", v.TicksCnt, " Loops:", v.LoopCnt)
		s += fmt.Sprint("Bytes  Received:", v.BytesReceived, "  Sent:", v.BytesSent, "\n")
		s += fmt.Sprint("Invs  Recieved:", v.InvsRecieved, "  Pending:", len(v.PendingInvs), "\n")
		s += fmt.Sprint("Bytes to send:", len(v.Send.Buf), " (", v.Send.MaxSize, " max)\n")
		s += fmt.Sprint("BlockInProgress:", len(v.GetBlockInProgress), "  GetHeadersInProgress:", v.GetHeadersInProgress, "\n")
		s += fmt.Sprint("GetBlocksDataNow:", v.GetBlocksDataNow, "  FetchNothing:", v.FetchNothing, "\n")
		s += fmt.Sprint("AllHeadersReceived:", v.AllHeadersReceived, "  HoldHeaders:", v.HoldHeaders, "\n")

		// Display ping stats
		s += fmt.Sprint("Ping history:")
		idx := v.PingHistoryIdx
		for _ = range(v.PingHistory) {
			s += fmt.Sprint(" ", v.PingHistory[idx])
			idx = (idx+1)%PingHistoryLength
		}

		s += fmt.Sprintln(" ->", v.GetAveragePing(), "ms")

		v.Mutex.Unlock()
	} else {
		s += fmt.Sprintln("Not yet connected")
	}
	return
}


func DropPeer(ip string) {
	c := Look4conn(ip)
	if c!=nil {
		c.DoS("FromUI")
		fmt.Println("The connection with", c.PeerAddr.Ip(), "is being dropped and the peer is banned")
	} else {
		fmt.Println("There is no such an active connection")
	}
}
