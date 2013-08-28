package main

import (
	"fmt"
	"sort"
	"strings"
	"net/http"
	"github.com/piotrnar/gocoin/btc"
)


func p_net(w http.ResponseWriter, r *http.Request) {
	if !ipchecker(r) {
		return
	}

	net_page := load_template("net.html")
	net_row := load_template("net_row.html")

	mutex.Lock()
	srt := make(sortedkeys, len(openCons))
	cnt := 0
	for k, v := range openCons {
		if !v.Broken {
			srt[cnt].key = k
			srt[cnt].ConnID = v.ConnID
			cnt++
		}
	}
	mutex.Unlock()
	sort.Sort(srt)
	net_page = strings.Replace(net_page, "{OUT_CONNECTIONS}", fmt.Sprint(OutConsActive), 1)
	net_page = strings.Replace(net_page, "{IN_CONNECTIONS}", fmt.Sprint(InConsActive), 1)
	net_page = strings.Replace(net_page, "{LISTEN_TCP}", fmt.Sprint(CFG.Net.ListenTCP, tcp_server_started), 1)
	net_page = strings.Replace(net_page, "{EXTERNAL_ADDR}", btc.NewNetAddr(BestExternalAddr()).String(), 1)

	for idx := range srt {
		v := openCons[srt[idx].key]
		s := net_row

		s = strings.Replace(s, "{CONNID}", fmt.Sprint(v.ConnID), -1)
		if v.Incoming {
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
		s = strings.Replace(s, "{TOTAL_RCVD}", bts(v.BytesReceived), 1)
		s = strings.Replace(s, "{TOTAL_SENT}", bts(v.BytesSent), 1)
		s = strings.Replace(s, "{NODE_VERSION}", fmt.Sprint(v.node.version), 1)
		s = strings.Replace(s, "{USER_AGENT}", v.node.agent, 1)
		if v.send.buf != nil {
			s = strings.Replace(s, "<!--SENDBUF-->", bts(uint64(len(v.send.buf))), 1)
		}
		if len(v.GetBlockInProgress)>0 {
			s = strings.Replace(s, "<!--BLKSINPROG-->", fmt.Sprint(len(v.GetBlockInProgress), "blks "), 1)
		}

		net_page = templ_add(net_page, "<!--PEER_ROW-->", s)
	}

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

	r.ParseForm()
	if len(r.Form["id"])==0 {
		fmt.Println("No id given")
		return
	}

	v := look2conn(r.Form["id"][0])
	if v == nil {
		fmt.Fprintln(w, "There is no such an active connection")
	} else {
		w.Write([]byte(node_stat(v)))
	}
}
