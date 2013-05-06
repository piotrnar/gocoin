package main                   

import (
	"flag"
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

	UploadLimit = flag.Uint("ul", 0, "Upload limit in KB/s (0 for no limit)")
	DownloadLimit = flag.Uint("dl", 0, "Download limit in KB/s (0 for no limit)")

	ul_last_sec int64
	ul_bytes_so_far int
	ul_bytes_prv_sec int
	ul_bytes_total uint64
)


func set_ulmax(par string) {
	v, e := strconv.ParseUint(par, 10, 64)
	if e == nil {
		*UploadLimit = uint(v<<10)
	}
	if *UploadLimit!=0 {
		fmt.Printf("Current upload limit is %d KB/s\n", *UploadLimit>>10)
	} else {
		fmt.Println("The upload speed is not limited")
	}
}


func set_dlmax(par string) {
	v, e := strconv.ParseUint(par, 10, 64)
	if e == nil {
		*DownloadLimit = uint(v<<10)
	}
	if *DownloadLimit!=0 {
		fmt.Printf("Current upload limit is %d KB/s\n", *DownloadLimit>>10)
	} else {
		fmt.Println("The upload speed is not limited")
	}
}


func bw_stats(par string) {
	fmt.Printf("Dowloading at %d KB/s.  Downloaded %d MB total\n",
		dl_bytes_prv_sec>>10, dl_bytes_total>>20)
	fmt.Printf("Uploading at %d KB/s.  Uploaded %d MB total.  Limit %d B/s\n",
		ul_bytes_prv_sec>>10, ul_bytes_total>>20, UploadLimit)
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
	if *DownloadLimit==0 {
		toread = len(buf)
	} else {
		do_recv(0)
		toread = int(*DownloadLimit) - dl_bytes_so_far
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
	if *UploadLimit==0 {
		tosend = len(buf)
	} else {
		do_sent(0)
		tosend = int(*UploadLimit) - ul_bytes_so_far
		if tosend > len(buf) {
			tosend = len(buf)
		}
	}
	bw_mutex.Unlock()
	n, e = con.Write(buf[:tosend])
	count_sent(n)
	return
}


