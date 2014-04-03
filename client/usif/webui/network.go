package webui

import (
	"fmt"
	"sort"
	"strings"
	"net/http"
	"github.com/piotrnar/gocoin/btc"
	"github.com/piotrnar/gocoin/client/common"
	"github.com/piotrnar/gocoin/client/network"
)


func p_net(w http.ResponseWriter, r *http.Request) {
	if !ipchecker(r) {
		return
	}

	net_page := load_template("net.html")
	net_row := load_template("net_row.html")

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
	net_page = strings.Replace(net_page, "{OUT_CONNECTIONS}", fmt.Sprint(network.OutConsActive), 1)
	net_page = strings.Replace(net_page, "{IN_CONNECTIONS}", fmt.Sprint(network.InConsActive), 1)
	net_page = strings.Replace(net_page, "{LISTEN_TCP}", fmt.Sprint(common.CFG.Net.ListenTCP, network.TCPServerStarted), 1)
	net_page = strings.Replace(net_page, "{EXTERNAL_ADDR}", btc.NewNetAddr(network.BestExternalAddr()).String(), 1)

	for idx := range srt {
		v := network.OpenCons[srt[idx].Key]
		s := net_row

		v.Mutex.Lock()
		s = strings.Replace(s, "{CONNID}", fmt.Sprint(v.ConnID), -1)
		if v.Incomming {
			s = strings.Replace(s, "{CONN_DIR_ICON}", "<img src=\"webui/incoming.png\">", 1)
		} else {
			s = strings.Replace(s, "{CONN_DIR_ICON}", "<img src=\"webui/outgoing.png\">", 1)
		}

		s = strings.Replace(s, "{PEER_ADDR}", v.PeerAddr.Ip(), 1)
		s = strings.Replace(s, "{PERR_PING}", fmt.Sprint(v.GetAveragePing()), 1)
		s = strings.Replace(s, "{LAST_RCVD_LEN}", fmt.Sprint(v.LastBtsRcvd), 1)
		s = strings.Replace(s, "{LAST_RCVD_CMD}", v.LastCmdRcvd, 1)
		s = strings.Replace(s, "{LAST_SENT_LEN}", fmt.Sprint(v.LastBtsSent), 1)
		s = strings.Replace(s, "{LAST_SENT_CNT}", v.LastCmdSent, 1)
		s = strings.Replace(s, "{TOTAL_RCVD}", common.BytesToString(v.BytesReceived), 1)
		s = strings.Replace(s, "{TOTAL_SENT}", common.BytesToString(v.BytesSent), 1)
		s = strings.Replace(s, "{NODE_VERSION}", fmt.Sprint(v.Node.Version), 1)
		s = strings.Replace(s, "{USER_AGENT}", v.Node.Agent, 1)
		if v.Send.Buf != nil {
			s = strings.Replace(s, "<!--SENDBUF-->", common.BytesToString(uint64(len(v.Send.Buf))), 1)
		}
		if len(v.GetBlockInProgress)>0 {
			s = strings.Replace(s, "<!--BLKSINPROG-->", fmt.Sprint(len(v.GetBlockInProgress), "blks "), 1)
		}

		v.Mutex.Unlock()

		net_page = templ_add(net_page, "<!--PEER_ROW-->", s)
	}
	network.Mutex_net.Unlock()

	write_html_head(w, r)
	w.Write([]byte(net_page))
	write_html_tail(w)
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
