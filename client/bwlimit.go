package main                   

import (
	"sync"
	"time"
	"fmt"
	"net"
	"strconv"
)

var (
	bw_mutex sync.Mutex

	dl_last_sec int64
	dl_bytes_so_far uint64
	dl_bytes_prv_sec uint64
	dl_bytes_total uint64

	UploadLimit uint64 // in bytes per second
	ul_last_sec int64
	ul_bytes_so_far uint64
	ul_bytes_prv_sec uint64
	ul_bytes_total uint64
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


func bw_stats(par string) {
	fmt.Printf("Dowloading at %d KB/s.  Downloaded %d MB total\n",
		dl_bytes_prv_sec>>10, dl_bytes_total>>20)
	fmt.Printf(" Uploading at %d KB/s.  Uploaded %d MB total.  Limit %d B/s\n",
		ul_bytes_prv_sec>>10, ul_bytes_total>>20, UploadLimit)
	return
}


func init() {
	newUi("bw", false, bw_stats, "Show network bandwidth statistics")
	newUi("maxu", false, set_ulmax, "Set maximum upload speed. The value is in KB/second")
}


func count_rcvd(n uint64) {
	bw_mutex.Lock()
	now := time.Now().Unix()
	if now != dl_last_sec {
		dl_bytes_prv_sec = dl_bytes_so_far
		dl_bytes_so_far = 0
		dl_last_sec = now
	}
	dl_bytes_so_far += n
	dl_bytes_total += n
	bw_mutex.Unlock()
}


func SockRead(con *net.TCPConn, buf []byte) (n int, e error) {
	n, e = con.Read(buf)
	if e == nil {
		count_rcvd(uint64(n))
	}
	return
}


func count_sent(n uint64) {
	bw_mutex.Lock()
	now := time.Now().Unix()
	if now != ul_last_sec {
		ul_bytes_prv_sec = ul_bytes_so_far
		ul_bytes_so_far = 0
		ul_last_sec = now
	}
	ul_bytes_so_far += n
	ul_bytes_total += n
	bw_mutex.Unlock()
}


// Send all the bytes, but respect the upload limit (force delays)
func SockWrite(con *net.TCPConn, buf []byte) (n int, e error) {
	n, e = con.Write(buf)
	if e == nil {
		count_sent(uint64(n))
	}
	return
/*
	var n, sent, left2send, now2send int
	var now int64
	for sent < len(buf) {
		now = time.Now().Unix()
		left2send = len(buf) - sent
		bw_mutex.Lock()
		if now!=ul_last_sec {
			ul_last_sec = now
			ul_bytes_so_far = 0
		}
		if UploadLimit==0 {
			now2send = left2send
		} else {
			now2send = int(UploadLimit-ul_bytes_so_far)
			if now2send > left2send {
				now2send = left2send
			}
		}
		ul_bytes_so_far += uint64(now2send)
		bw_mutex.Unlock()
		
		if now2send>0 {
			n, e = con.Write(buf[sent:sent+now2send])
			if e != nil {
				return
			}
			sent += n
		}
		time.Sleep(250e6) // wait 250ms
	}
	return
*/
}


