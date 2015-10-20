package webui

import (
	"fmt"
	"sort"
	"strings"
	"net/http"
	"encoding/json"
	"github.com/piotrnar/gocoin/lib/btc"
	"github.com/piotrnar/gocoin/client/common"
	"github.com/piotrnar/gocoin/client/network"
)


func p_net(w http.ResponseWriter, r *http.Request) {
	if !ipchecker(r) {
		return
	}

	net_page := load_template("net.html")

	network.Mutex_net.Lock()
	net_page = strings.Replace(net_page, "{LISTEN_TCP}", fmt.Sprint(common.IsListenTCP(), network.TCPServerStarted), 1)
	net_page = strings.Replace(net_page, "{EXTERNAL_ADDR}", btc.NewNetAddr(network.BestExternalAddr()).String(), 1)

	network.Mutex_net.Unlock()

	write_html_head(w, r)
	w.Write([]byte(net_page))
	write_html_tail(w)
}


type one_net_con struct {
	Id uint32
	Incomming bool
	PeerIp string
	Ping int
	LastBtsRcvd uint32
	LastCmdRcvd string
	LastBtsSent uint32
	LastCmdSent string
	BytesReceived, BytesSent uint64
	Node struct {
		Version uint32
		Services uint64
		Timestamp uint64
		Height uint32
		Agent string
		DoNotRelayTxs bool
		ReportedIp4 uint32
	}
	SendBufLen int
	BlksInProgress int
}

func json_netcon(w http.ResponseWriter, r *http.Request) {
	if !ipchecker(r) {
		return
	}

	network.Mutex_net.Lock()
	srt := make(network.SortedKeys, len(network.OpenCons))
	cnt := 0
	for k, v := range network.OpenCons {
		if !v.IsBroken() {
			srt[cnt].Key = k
			srt[cnt].ConnID = v.ConnID
			cnt++
		}
	}
	sort.Sort(srt)

	net_cons := make([]one_net_con, cnt)

	for idx := range srt {
		v := network.OpenCons[srt[idx].Key]

		v.Mutex.Lock()
		net_cons[idx].Id = v.ConnID
		net_cons[idx].Incomming = v.Incoming
		net_cons[idx].PeerIp = v.PeerAddr.Ip()
		net_cons[idx].Ping = v.GetAveragePing()
		net_cons[idx].LastBtsRcvd = v.LastBtsRcvd
		net_cons[idx].LastCmdRcvd = v.LastCmdRcvd
		net_cons[idx].LastBtsSent = v.LastBtsSent
		net_cons[idx].LastCmdSent = v.LastCmdSent
		net_cons[idx].BytesReceived = v.BytesReceived
		net_cons[idx].BytesSent = v.BytesSent
		net_cons[idx].Node = v.Node
		net_cons[idx].SendBufLen = len(v.Send.Buf)
		net_cons[idx].BlksInProgress = len(v.GetBlockInProgress)
		v.Mutex.Unlock()
	}
	network.Mutex_net.Unlock()

	bx, er := json.Marshal(net_cons)
	if er == nil {
		w.Header()["Content-Type"] = []string{"application/json"}
		w.Write(bx)
	} else {
		println(er.Error())
	}

}


func raw_net(w http.ResponseWriter, r *http.Request) {
	if !ipchecker(r) {
		return
	}

	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintln(w, "Error")
		}
	}()

	if len(r.Form["id"])==0 {
		fmt.Println("No id given")
		return
	}

	v := network.Look4conn(r.Form["id"][0])
	if v == nil {
		fmt.Fprintln(w, "There is no such an active connection")
	} else {
		w.Write([]byte(v.Stats()))
	}
}
