package main                   

import (
	"sync"
	"time"
	"fmt"
	"net"
	"strconv"
)

const secondsBack = 10  // timespan on which we measure the speed


var (
	bw_mutex sync.Mutex

	tot_up, tot_dn uint64

	siz_sent map[uint32]uint64 = make(map[uint32]uint64, secondsBack)
	sec_sent uint32

	siz_rcvd map[uint32]uint64 = make(map[uint32]uint64, secondsBack)
	sec_rcvd uint32

	UploadLimit uint64 // in bytes per second
	last_sec int64
	bytes_so_far uint64
)


func set_ulmax(par string) {
	v, e := strconv.ParseUint(par, 10, 64)
	if e == nil {
		UploadLimit = v<<10
	}
	if UploadLimit!=0 {
		fmt.Printf("Current upload limit is %d KB/s\n", UploadLimit>>10)
	} else {
		fmt.Println("The upload speed is not limited")
	}
}


func init() {
	newUi("maxu", false, set_ulmax, "Set maximum upload speed. The value is in KB/second")
	go expire_records()
}


func SockRead(con *net.TCPConn, buf []byte) (n int, e error) {
	n, e = con.Read(buf)
	bw_got(n)
	return
}


// Send all the bytes, but respect the upload limit (force delays)
func SockWrite(con *net.TCPConn, buf []byte) (e error) {
	var n, sent, left2send, now2send int
	var now int64
	for sent < len(buf) {
		now = time.Now().Unix()
		left2send = len(buf) - sent
		bw_mutex.Lock()
		if now!=last_sec {
			last_sec = now
			bytes_so_far = 0
		}
		if UploadLimit==0 {
			now2send = left2send
		} else {
			now2send = int(UploadLimit-bytes_so_far)
			if now2send > left2send {
				now2send = left2send
			}
		}
		bytes_so_far += uint64(now2send)
		bw_mutex.Unlock()
		
		if now2send>0 {
			n, e = con.Write(buf[sent:sent+now2send])
			if e != nil {
				return
			}
			sent += n
			bw_sent(n)
		}
		time.Sleep(250e6) // wait 250ms
	}
	return
}


func expire_records() {
	for {
		now := uint32(time.Now().Unix())
		bw_mutex.Lock()
		for k, _ := range siz_sent {
			if k<now-secondsBack {
				delete(siz_sent, k)
			}
		}
		for k, _ := range siz_rcvd {
			if k<now-secondsBack {
				delete(siz_rcvd, k)
			}
		}
		bw_mutex.Unlock()
		time.Sleep(1e9)
	}
}


func bw_sent(siz int) {
	tot_up += uint64(siz)
	now := uint32(time.Now().Unix())
	if cv, ok := siz_sent[now]; ok {
		// same second
		siz_sent[now] = cv+uint64(siz)
	} else {
		// new second
		siz_sent[now] = uint64(siz)
	}
}

func bw_got(siz int) {
	bw_mutex.Lock()
	tot_dn += uint64(siz)
	now := uint32(time.Now().Unix())
	if cv, ok := siz_rcvd[now]; ok {
		// same second
		siz_rcvd[now] = cv+uint64(siz)
	} else {
		// new second
		siz_rcvd[now] = uint64(siz)
		// remove expired entries
	}
	bw_mutex.Unlock()
}


func bw_stats() (s string) {
	bw_mutex.Lock()
	
	now := uint32(time.Now().Unix())
	sum := uint64(0)  // 60 seconds average
	for i := now-secondsBack; i<=now; i++ {
		if v, ok := siz_rcvd[i]; ok {
			sum += v
		}
	}
	s += fmt.Sprintf("DOWN:[%d KB/s,  %d MB tot]  ", (sum/secondsBack)>>10, tot_dn>>20)
	
	sum = 0
	for i := now-secondsBack; i<=now; i++ {
		if v, ok := siz_sent[i]; ok {
			sum += v
		}
	}
	s += fmt.Sprintf(" UP:[%d KB/s, %d MB tot]", (sum/secondsBack)>>10, tot_up>>20)

	bw_mutex.Unlock()
	return
}
