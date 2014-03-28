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
	if v.Incomming {
		s += fmt.Sprintln("Comming from", v.PeerAddr.Ip())
	} else {
		s += fmt.Sprintln("Going to", v.PeerAddr.Ip())
	}
	if !v.ConnectedAt.IsZero() {
		v.Mutex.Lock()
		s += fmt.Sprintln("Connected at", v.ConnectedAt.Format("2006-01-02 15:04:05"))
		if v.Node.Version!=0 {
			s += fmt.Sprintln("Node common.Version:", v.Node.Version)
			s += fmt.Sprintln("User Agent:", v.Node.Agent)
			s += fmt.Sprintln("Chain Height:", v.Node.Height)
			s += fmt.Sprintf("Reported IP: %d.%d.%d.%d", byte(v.Node.ReportedIp4>>24), byte(v.Node.ReportedIp4>>16),
				byte(v.Node.ReportedIp4>>8), byte(v.Node.ReportedIp4))
		}
		s += fmt.Sprintln("Last data got:", time.Now().Sub(v.LastDataGot).String())
		s += fmt.Sprintln("Last data sent:", time.Now().Sub(v.Send.LastSent).String())
		s += fmt.Sprintln("Last command received:", v.LastCmdRcvd, " ", v.LastBtsRcvd, "bytes")
		s += fmt.Sprintln("Last command sent:", v.LastCmdSent, " ", v.LastBtsSent, "bytes")
		s += fmt.Sprintln("Bytes received:", v.BytesReceived)
		s += fmt.Sprintln("Bytes sent:", v.BytesSent)
		s += fmt.Sprintln("Invs recieved:", v.InvsRecieved)
		s += fmt.Sprintln("Next getbocks sending in", v.NextBlocksAsk.Sub(time.Now()).String())
		if v.LastBlocksFrom != nil {
			s += fmt.Sprintln("Last block asked:", v.LastBlocksFrom.Height, v.LastBlocksFrom.BlockHash.String())
		}
		s += fmt.Sprintln("Ticks:", v.TicksCnt, " Loops:", v.LoopCnt)
		if v.Send.Buf != nil {
			s += fmt.Sprintln("Bytes to send:", len(v.Send.Buf))
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

		v.Mutex.Unlock()
	} else {
		s += fmt.Sprintln("Not yet connected")
	}
	return
}


func DropPeer(ip string) {
	c := Look4conn(ip)
	if c!=nil {
		c.DoS()
		fmt.Println("The connection with", c.PeerAddr.Ip(), "is being dropped and the peer is banned")
	} else {
		fmt.Println("There is no such an active connection")
	}
}
