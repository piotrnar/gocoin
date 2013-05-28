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
	dl_bytes_so_far int

	dl_bytes_prv_sec, dl_bytes_priod uint64
	dl_bytes_total uint64

	UploadLimit uint
	DownloadLimit uint

	ul_last_sec int64
	ul_bytes_so_far int
	ul_bytes_prv_sec, ul_bytes_priod uint64
	ul_bytes_total uint64
)


func set_ulmax(par string) {
	v, e := strconv.ParseUint(par, 10, 64)
	if e == nil {
		UploadLimit = uint(v<<10)
	}
	if UploadLimit!=0 {
		fmt.Printf("Current upload limit is %d KB/s\n", UploadLimit>>10)
	} else {
		fmt.Println("The upload speed is not limited")
	}
}


func set_dlmax(par string) {
	v, e := strconv.ParseUint(par, 10, 64)
	if e == nil {
		DownloadLimit = uint(v<<10)
	}
	if DownloadLimit!=0 {
		fmt.Printf("Current upload limit is %d KB/s\n", DownloadLimit>>10)
	} else {
		fmt.Println("The upload speed is not limited")
	}
}


func bw_stats() {
	bw_mutex.Lock()
	tick_recv()
	tick_sent()
	fmt.Printf("Downloading at %d/%d KB/s, %s total",
		dl_bytes_prv_sec>>10, DownloadLimit>>10, bts(dl_bytes_total))
	fmt.Printf("  |  Uploading at %d/%d KB/s, %s total\n",
		ul_bytes_prv_sec>>10, UploadLimit>>10, bts(ul_bytes_total))
	bw_mutex.Unlock()
	return
}


func init() {
	newUi("ulimit ul", false, set_ulmax, "Set maximum upload speed. The value is in KB/second - 0 for unlimited")
	newUi("dlimit dl", false, set_dlmax, "Set maximum download speed. The value is in KB/second - 0 for unlimited")
}


func tick_recv() {
	now := time.Now().Unix()
	if now != dl_last_sec {
		if now - dl_last_sec == 1 {
			dl_bytes_prv_sec = dl_bytes_priod
		} else {
			dl_bytes_prv_sec = 0
		}
		dl_bytes_priod = 0
		dl_bytes_so_far = 0
		dl_last_sec = now
	}
}


func tick_sent() {
	now := time.Now().Unix()
	if now != ul_last_sec {
		if now - ul_last_sec == 1 {
			ul_bytes_prv_sec = ul_bytes_priod
		} else {
			ul_bytes_prv_sec = 0
		}
		ul_bytes_priod = 0
		ul_bytes_so_far = 0
		ul_last_sec = now
	}
}


func SockRead(con *net.TCPConn, buf []byte) (n int, e error) {
	var toread int
	bw_mutex.Lock()
	tick_recv()
	if DownloadLimit==0 {
		toread = len(buf)
	} else {
		toread = int(DownloadLimit) - dl_bytes_so_far
		if toread > len(buf) {
			toread = len(buf)
		} else if toread < 0 {
			toread = 0
		}
	}
	dl_bytes_so_far += toread
	bw_mutex.Unlock()

	if toread>0 {
		// Wait 1 millisecond for a data, timeout if nothing there
		con.SetReadDeadline(time.Now().Add(time.Millisecond))
		n, e = con.Read(buf[:toread])
		bw_mutex.Lock()
		dl_bytes_total += uint64(n)
		dl_bytes_priod += uint64(n)
		bw_mutex.Unlock()
	} else {
		// supsend a task for awhile, to prevent stucking in a busy loop
		time.Sleep(10*time.Millisecond)
	}
	return
}


// Send all the bytes, but respect the upload limit (force delays)
func SockWrite(con *net.TCPConn, buf []byte) (n int, e error) {
	var tosend int
	bw_mutex.Lock()
	tick_sent()
	if UploadLimit==0 {
		tosend = len(buf)
	} else {
		tosend = int(UploadLimit) - ul_bytes_so_far
		if tosend > len(buf) {
			tosend = len(buf)
		} else if tosend<0 {
			tosend = 0
		}
	}
	ul_bytes_so_far += tosend
	bw_mutex.Unlock()
	if tosend > 0 {
		// Set timeout to prevent thread from getting stuck if the other end does not read
		con.SetWriteDeadline(time.Now().Add(time.Millisecond))
		n, e = con.Write(buf[:tosend])
		bw_mutex.Lock()
		ul_bytes_total += uint64(n)
		ul_bytes_priod += uint64(n)
		bw_mutex.Unlock()
		if e != nil {
			if nerr, ok := e.(net.Error); ok && nerr.Timeout() {
				e = nil
			}
		}
	} else {
		time.Sleep(10*time.Millisecond)
	}
	return
}
