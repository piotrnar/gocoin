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
	dl_bytes_prv_sec int
	dl_bytes_total uint64

	UploadLimit uint
	DownloadLimit uint

	ul_last_sec int64
	ul_bytes_so_far int
	ul_bytes_prv_sec int
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


func bw_stats(par string) {
	bw_mutex.Lock()
	do_recv(0)
	do_sent(0)
	fmt.Printf("Downloading at %d KB/s. \tDownloaded %d MB total. \tLimit %d KB/s\n",
		dl_bytes_prv_sec>>10, dl_bytes_total>>20, DownloadLimit>>10)
	fmt.Printf("Uploading at %d KB/s.  \tUploaded   %d MB total. \tLimit %d KB/s\n",
		ul_bytes_prv_sec>>10, ul_bytes_total>>20, UploadLimit>>10)
	bw_mutex.Unlock()
	return
}


func init() {
	newUi("bw", false, bw_stats, "Show network bandwidth statistics")
	newUi("ulimit ul", false, set_ulmax, "Set maximum upload speed. The value is in KB/second - 0 for unlimited")
	newUi("dlimit dl", false, set_dlmax, "Set maximum download speed. The value is in KB/second - 0 for unlimited")
}


func do_recv(n int) {
	now := time.Now().Unix()
	if now != dl_last_sec {
		dl_bytes_prv_sec = dl_bytes_so_far
		dl_bytes_so_far = 0
		dl_last_sec = now
	}
	dl_bytes_so_far += n
	dl_bytes_total += uint64(n)
}


func count_rcvd(n int) {
	bw_mutex.Lock()
	do_recv(n)
	bw_mutex.Unlock()
}


func SockRead(con *net.TCPConn, buf []byte) (n int, e error) {
	var toread int
	bw_mutex.Lock()
	if DownloadLimit==0 {
		toread = len(buf)
	} else {
		do_recv(0)
		toread = int(DownloadLimit) - dl_bytes_so_far
		if toread > len(buf) {
			toread = len(buf)
		}
	}
	bw_mutex.Unlock()

	if toread>0 {
		n, e = con.Read(buf[:toread])
		count_rcvd(n)
	}
	return
}


func do_sent(n int) {
	now := time.Now().Unix()
	if now != ul_last_sec {
		ul_bytes_prv_sec = ul_bytes_so_far
		ul_bytes_so_far = 0
		ul_last_sec = now
	}
	ul_bytes_so_far += n
	ul_bytes_total += uint64(n)
}


func count_sent(n int) {
	bw_mutex.Lock()
	do_sent(n)
	bw_mutex.Unlock()
}


// Send all the bytes, but respect the upload limit (force delays)
func SockWrite(con *net.TCPConn, buf []byte) (n int, e error) {
	var tosend int
	bw_mutex.Lock()
	if UploadLimit==0 {
		tosend = len(buf)
	} else {
		do_sent(0)
		tosend = int(UploadLimit) - ul_bytes_so_far
		if tosend > len(buf) {
			tosend = len(buf)
		}
	}
	bw_mutex.Unlock()
	if tosend > 0 {
		n, e = con.Write(buf[:tosend])
		count_sent(n)
	}
	return
}


