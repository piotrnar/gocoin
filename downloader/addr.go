package main

import (
	"os"
	"fmt"
	"net"
	"sync"
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
			fmt.Println("parse_addr:", n, e)
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


func add_ip_str(s string) bool {
	ip := net.ParseIP(s)
	if len(ip)==16 {
		var ip4 [4]byte
		copy(ip4[:], ip[12:16])
		if len(AddrDatbase)==0 {
			FirstIp = ip4
		}
		AddrDatbase[ip4] = false
		return true
	} else {
		fmt.Println("IP syntax error:", s)
		return false
	}
}

func load_ips() {
	f, er := os.Open("ips.txt")
	if er != nil {
		fmt.Println(er.Error())
		fmt.Println("You can store more seed peers in file named ips.txt (execute 's' to save there the currently connected peers)")
		return
	}
	rd := bufio.NewReader(f)
	for {
		d, _, er := rd.ReadLine()
		if er != nil {
			break
		}
		add_ip_str(string(d))
	}
	f.Close()
}

func open_connection_count() (res uint) {
	open_connection_mutex.Lock()
	res = uint(len(open_connection_list))
	open_connection_mutex.Unlock()
	return
}
