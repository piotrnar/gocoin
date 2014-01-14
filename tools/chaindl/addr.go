package main

import (
	"os"
	"fmt"
	"net"
	"sync"
	"time"
	"bytes"
	"bufio"
	"github.com/piotrnar/gocoin/btc"
)

var (
	FirstIp [4]byte
	AddrMutex sync.Mutex
	AddrDatbase map[[4]byte]bool = make(map[[4]byte]bool) // true - if is conencted
)

func validip4(ip []byte) bool {
	// local host
	if ip[0]==0 || ip[0]==127 {
		return false
	}

	// RFC1918
	if ip[0]==10 || ip[0]==192 && ip[1]==168 || ip[0]==172 && ip[1]>=16 && ip[1]<=31 {
		return false
	}

	//RFC3927
	if ip[0]==169 && ip[1]==254 {
		return false
	}

	return true
}


func parse_addr(pl []byte) {
	b := bytes.NewBuffer(pl)
	cnt, _ := btc.ReadVLen(b)
	for i := 0; i < int(cnt); i++ {
		var buf [30]byte
		var ip4 [4]byte
		n, e := b.Read(buf[:])
		if n!=len(buf) || e!=nil {
			println("parse_addr:", n, e)
			break
		}
		copy(ip4[:], buf[24:28])
		if validip4(ip4[:]) {
			AddrMutex.Lock()
			if _, pres := AddrDatbase[ip4]; !pres {
				AddrDatbase[ip4] = false
			}
			AddrMutex.Unlock()
		}
	}
}


func load_ips() {
	f, er := os.Open("ips.txt")
	if er != nil {
		println(er.Error())
		os.Exit(1)
	}
	defer f.Close()
	rd := bufio.NewReader(f)
	AddrMutex.Lock()
	defer AddrMutex.Unlock()
	for {
		d, _, er := rd.ReadLine()
		if er != nil {
			break
		}
		ip := net.ParseIP(string(d))
		if len(ip)==16 {
			var ip4 [4]byte
			copy(ip4[:], ip[12:16])
			if len(AddrDatbase)==0 {
				FirstIp = ip4
			}
			AddrDatbase[ip4] = false
		} else {
			println("IP syntax error:", string(d))
		}
	}
}

func save_peers() {
	f, _ := os.Create("ips.txt")
	fmt.Fprintf(f, "%d.%d.%d.%d\n", FirstIp[0], FirstIp[1], FirstIp[2], FirstIp[3])
	ccc := 1
	AddrMutex.Lock()
	for k, _ := range AddrDatbase {
		if k!=FirstIp {
			fmt.Fprintf(f, "%d.%d.%d.%d\n", k[0], k[1], k[2], k[3])
			ccc++
		}
	}
	AddrMutex.Unlock()
	f.Close()
}


func open_connection_count() (res int) {
	open_connection_mutex.Lock()
	res = len(open_connection_list)
	open_connection_mutex.Unlock()
	return
}


func drop_worst_peers() {
	if open_connection_count()<MAX_CONNECTIONS {
		return
	}
	open_connection_mutex.Lock()

	var min_bps float64
	var minbps_rec *one_net_conn
	for _, v := range open_connection_list {
		if v.isbroken() {
			// alerady broken
			continue
		}

		if v.connected_at.IsZero() {
			// still connecting
			continue
		}

		if time.Now().Sub(v.connected_at) < 3*time.Second {
			// give him 3 seconds
			continue
		}

		v.Lock()
		br := v.bytes_received
		v.Unlock()

		if br==0 {
			// if zero bytes received after 3 seconds - drop it!
			v.setbroken(true)
			//println(" -", v.peerip, "- idle")
			COUNTER("DROP_IDLE")
			continue
		}

		bps := v.bps()
		if minbps_rec==nil || bps<min_bps {
			minbps_rec = v
			min_bps = bps
		}
	}
	if minbps_rec!=nil {
		//fmt.Printf(" - %s - slowest (%.3f KBps, %d KB)\n", minbps_rec.peerip, min_bps/1e3, minbps_rec.bytes_received>>10)
		COUNTER("DROP_SLOW")
		minbps_rec.setbroken(true)
	}

	open_connection_mutex.Unlock()
}
/*
*/
