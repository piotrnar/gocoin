package webui

import (
	"encoding/json"
	"fmt"
	"net/http"
	"runtime/debug"
	"strconv"

	"github.com/piotrnar/gocoin/client/common"
	"github.com/piotrnar/gocoin/client/network"
	"github.com/piotrnar/gocoin/client/txpool"
)

// Takes a snapshot of the stats of all the open connections.
// Note: never write anything to the network (nor to stdout) while holding Mutex_net.
func get_net_cons() (net_cons []network.ConnInfo) {
	network.Mutex_net.Lock()
	defer network.Mutex_net.Unlock()

	net_cons = make([]network.ConnInfo, len(network.OpenCons))
	tmp, _ := network.GetSortedConnections()
	i := len(net_cons)
	for _, v := range tmp {
		i--
		v.Conn.GetStats(&net_cons[i])
		net_cons[i].HasImmunity = v.MinutesOnline < network.ImmunityMinutes()
	}
	return
}

func json_netcon(w http.ResponseWriter, r *http.Request) {
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

	bx, er := json.Marshal(get_net_cons())
	if er == nil {
		w.Header()["Content-Type"] = []string{"application/json"}
		w.Write(bx)
	} else {
		println(er.Error())
	}

}

func json_peerst(w http.ResponseWriter, r *http.Request) {
	if len(r.Form["id"]) == 0 {
		return
	}

	conid, e := strconv.ParseUint(r.Form["id"][0], 10, 32)
	if e != nil {
		return
	}

	if v := network.GetConnFromID(uint32(conid)); v != nil {
		var res network.ConnInfo
		v.GetStats(&res)
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
	type one_ext_ip struct {
		Ip               string
		Count, Timestamp uint
	}

	var out struct {
		ExternalIP       []one_ext_ip
		Dl_speed_now     uint64
		Ul_total         uint64
		Open_conns_total int
		Dl_speed_max     uint64
		Dl_total         uint64
		Ul_speed_now     uint64
		Ul_speed_max     uint64
		GetMPConnID      int
		Open_conns_out   uint32
		Open_conns_in    uint32
		DefaultTCPPort   uint16
		GetMPInProgress  bool
		ListenTCPOn      bool
		TXPServerStarted bool
	}

	common.LockBw()
	common.TickRecv()
	common.TickSent()
	out.Dl_speed_now = common.GetAvgBW(common.DlBytesPrevSec[:], common.DlBytesPrevSecIdx, 5)
	out.Dl_speed_max = common.DownloadLimit()
	out.Dl_total = common.DlBytesTotal
	out.Ul_speed_now = common.GetAvgBW(common.UlBytesPrevSec[:], common.UlBytesPrevSecIdx, 5)
	out.Ul_speed_max = common.UploadLimit()
	out.Ul_total = common.UlBytesTotal
	common.UnlockBw()

	network.Mutex_net.Lock()
	out.Open_conns_total = len(network.OpenCons)
	out.Open_conns_out = network.OutConsActive
	out.Open_conns_in = network.InConsActive
	out.TXPServerStarted = network.TCPServerStarted
	network.Mutex_net.Unlock()
	out.ListenTCPOn = common.IsListenTCP()
	out.DefaultTCPPort = common.ConfiguredTcpPort()

	for _, rec := range network.GetExternalIPs() {
		out.ExternalIP = append(out.ExternalIP, one_ext_ip{
			Ip:    fmt.Sprintf("%d.%d.%d.%d", byte(rec.IP>>24), byte(rec.IP>>16), byte(rec.IP>>8), byte(rec.IP)),
			Count: rec.Cnt, Timestamp: rec.Tim})
	}

	out.GetMPInProgress = len(txpool.GetMPInProgressTicket) != 0
	out.GetMPConnID = network.GetMPInProgressConnID.Get()

	bx, er := json.Marshal(out)
	if er == nil {
		w.Header()["Content-Type"] = []string{"application/json"}
		w.Write(bx)
	} else {
		println(er.Error())
	}
}

func json_bwchar(w http.ResponseWriter, r *http.Request) {
	var cnt uint64

	if len(r.Form["seconds"]) > 0 {
		cnt, _ = strconv.ParseUint(r.Form["seconds"][0], 10, 32)
	}

	var dl, ul common.BwChartDat
	common.GetBwChart(int64(cnt), &dl, &ul)

	var out struct {
		DL           [common.BwChartSlots]uint64 // max 200 records (from 200 seconds to ~16.7 hours)
		UL           [common.BwChartSlots]uint64
		MaxDL, MaxUL uint64
	}
	out.DL = dl.Avg
	out.UL = ul.Avg
	for i := 0; i < common.BwChartSlots; i++ {
		if dl.Max[i] > out.MaxDL {
			out.MaxDL = dl.Max[i]
		}
		if ul.Max[i] > out.MaxUL {
			out.MaxUL = ul.Max[i]
		}
	}

	bx, er := json.Marshal(out)
	if er == nil {
		w.Header()["Content-Type"] = []string{"application/json"}
		w.Write(bx)
	} else {
		println(er.Error())
	}
}
