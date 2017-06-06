package webui

import (
	"fmt"
	"sort"
	"strings"
	"strconv"
	"net/http"
	"encoding/json"
	"runtime/debug"
	"github.com/piotrnar/gocoin/lib/btc"
	"github.com/piotrnar/gocoin/client/common"
	"github.com/piotrnar/gocoin/client/network"
)

type sorted_net_cons []network.ConnInfo

func (sk sorted_net_cons) Len() int {
	return len(sk)
}

func (sk sorted_net_cons) Less(a, b int) bool {
	return sk[a].ID<sk[b].ID
}

func (sk sorted_net_cons) Swap(a, b int) {
	sk[a], sk[b] = sk[b], sk[a]
}



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


func json_netcon(w http.ResponseWriter, r *http.Request) {
	if !ipchecker(r) {
		return
	}

	defer func() {
		if r := recover(); r != nil {
			err, ok := r.(error)
			if !ok {
				err = fmt.Errorf("pkg: %v", r)
			}
			fmt.Println("json_netcon recovered:", err.Error())
			fmt.Println(string(debug.Stack()))
		}
	}()

	network.Mutex_net.Lock()
	defer network.Mutex_net.Unlock()

	net_cons := make(sorted_net_cons, len(network.OpenCons))
	var i int
	for _, v := range network.OpenCons {
		v.GetStats(&net_cons[i])
		i++
	}
	sort.Sort(net_cons)

	bx, er := json.Marshal(net_cons)
	if er == nil {
		w.Header()["Content-Type"] = []string{"application/json"}
		w.Write(bx)
	} else {
		println(er.Error())
	}

}


func json_peerst(w http.ResponseWriter, r *http.Request) {
	if !ipchecker(r) {
		return
	}

	if len(r.Form["id"])==0 {
		return
	}

	conid, e := strconv.ParseUint(r.Form["id"][0], 10, 32)
	if e != nil {
		return
	}

	var res *network.ConnInfo

	network.Mutex_net.Lock()
	for _, v := range network.OpenCons {
		if uint32(conid)==v.ConnID {
			res = new(network.ConnInfo)
			v.GetStats(res)
			break
		}
	}
	network.Mutex_net.Unlock()

	if res != nil {
		bx, er := json.Marshal(&res)
		if er == nil {
			w.Header()["Content-Type"] = []string{"application/json"}
			w.Write(bx)
		} else {
			println(er.Error())
		}
	}
}


func json_bwidth(w http.ResponseWriter, r *http.Request) {
	if !ipchecker(r) {
		return
	}

	type one_ext_ip struct {
		Ip string
		Count, Timestamp uint
	}

	var out struct {
		Open_conns_total int
		Open_conns_out uint32
		Open_conns_in uint32
		Dl_speed_now uint64
		Dl_speed_max uint
		Dl_total uint64
		Ul_speed_now uint64
		Ul_speed_max uint
		Ul_total uint64
		ExternalIP []one_ext_ip
	}

	common.LockBw()
	common.TickRecv()
	common.TickSent()
	out.Dl_speed_now = common.GetAvgBW(common.DlBytesPrevSec[:], common.DlBytesPrevSecIdx, 5)
	out.Dl_speed_max = common.DownloadLimit
	out.Dl_total = common.DlBytesTotal
	out.Ul_speed_now = common.GetAvgBW(common.UlBytesPrevSec[:], common.UlBytesPrevSecIdx, 5)
	out.Ul_speed_max = common.UploadLimit
	out.Ul_total = common.UlBytesTotal
	common.UnlockBw()

	network.Mutex_net.Lock()
	out.Open_conns_total = len(network.OpenCons)
	out.Open_conns_out = network.OutConsActive
	out.Open_conns_in = network.InConsActive
	network.Mutex_net.Unlock()

	arr := network.GetExternalIPs()
	for _, rec := range arr {
		out.ExternalIP = append(out.ExternalIP, one_ext_ip{
			Ip : fmt.Sprintf("%d.%d.%d.%d", byte(rec.IP>>24), byte(rec.IP>>16), byte(rec.IP>>8), byte(rec.IP)),
			Count:rec.Cnt, Timestamp:rec.Tim})
	}

	bx, er := json.Marshal(out)
	if er == nil {
		w.Header()["Content-Type"] = []string{"application/json"}
		w.Write(bx)
	} else {
		println(er.Error())
	}
}

func json_bwchar(w http.ResponseWriter, r *http.Request) {
	if !ipchecker(r) {
		return
	}
	var out struct {
		DL [0x100]uint64
		UL [0x100]uint64
	}

	common.LockBw()
	common.TickRecv()
	common.TickSent()

	idx := common.DlBytesPrevSecIdx
	for i:=0; i<0x100; i++ {
		if idx==0 {
			idx = 0xff
		} else {
			idx--
		}
		out.DL[i] = common.DlBytesPrevSec[idx]
	}

	idx = common.UlBytesPrevSecIdx
	for i:=0; i<0x100; i++ {
		if idx==0 {
			idx = 0xff
		} else {
			idx--
		}
		out.UL[i] = common.UlBytesPrevSec[idx]
	}

	common.UnlockBw()

	bx, er := json.Marshal(out)
	if er == nil {
		w.Header()["Content-Type"] = []string{"application/json"}
		w.Write(bx)
	} else {
		println(er.Error())
	}
}
