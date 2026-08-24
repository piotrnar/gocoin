package common

import (
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

var (
	bw_mutex sync.Mutex

	dl_last_sec     int64 = time.Now().Unix()
	dl_bytes_so_far int

	DlBytesPrevSec    [0x10000]uint64 // this buffer takes 524288 bytes (hope it's not a problem)
	DlBytesPrevSecIdx uint16

	dl_bytes_priod uint64
	DlBytesTotal   uint64

	upload_limit   uint64
	download_limit uint64

	ul_last_sec     int64 = time.Now().Unix()
	ul_bytes_so_far int

	UlBytesPrevSec    [0x10000]uint64 // this buffer takes 524288 bytes (hope it's not a problem)
	UlBytesPrevSecIdx uint16
	ul_bytes_priod    uint64
	UlBytesTotal      uint64
)

func TickRecv() (ms int) {
	tn := time.Now()
	ms = tn.Nanosecond() / 1e6
	now := tn.Unix()
	if now < dl_last_sec {
		dl_last_sec = now // This is to prevent a lock-up when OS clock is updated back
		ms = 1e6 - 1
	}
	if now != dl_last_sec {
		for now-dl_last_sec != 1 {
			DlBytesPrevSec[DlBytesPrevSecIdx] = 0
			DlBytesPrevSecIdx++
			dl_last_sec++
		}
		DlBytesPrevSec[DlBytesPrevSecIdx] = dl_bytes_priod
		DlBytesPrevSecIdx++
		dl_bytes_priod = 0
		dl_bytes_so_far = 0
		dl_last_sec = now
	}
	return
}

func TickSent() (ms int) {
	tn := time.Now()
	ms = tn.Nanosecond() / 1e6
	now := tn.Unix()
	if now < ul_last_sec {
		ul_last_sec = now // This is to prevent a lock-up when OS clock is updated back
		ms = 1e6 - 1
	}
	if now != ul_last_sec {
		var loop_cnt int
		for now-ul_last_sec != 1 {
			UlBytesPrevSec[UlBytesPrevSecIdx] = 0
			UlBytesPrevSecIdx++
			ul_last_sec++
			loop_cnt++
		}
		UlBytesPrevSec[UlBytesPrevSecIdx] = ul_bytes_priod
		UlBytesPrevSecIdx++
		ul_bytes_priod = 0
		ul_bytes_so_far = 0
		ul_last_sec = now
	}
	return
}

// SockRead reads the given number of bytes, but respecting the download limit.
// Returns -1 and no error if we can't read any data now, because of bw limit.
func SockRead(con net.Conn, buf []byte) (n int, e error) {
	var toread int
	bw_mutex.Lock()
	ms := TickRecv()
	if DownloadLimit() == 0 {
		toread = len(buf)
	} else {
		toread = ms*int(DownloadLimit())/1000 - dl_bytes_so_far
		if toread > len(buf) {
			toread = len(buf)
			if toread > 4096 {
				toread = 4096
			}
		} else if toread < 0 {
			toread = 0
		}
	}
	dl_bytes_so_far += toread
	bw_mutex.Unlock()

	if toread > 0 {
		// Wait 10 millisecond for a data, timeout if nothing there
		con.SetReadDeadline(time.Now().Add(10 * time.Millisecond))
		n, e = con.Read(buf[:toread])
		bw_mutex.Lock()
		dl_bytes_so_far -= toread
		if n > 0 {
			dl_bytes_so_far += n
			DlBytesTotal += uint64(n)
			dl_bytes_priod += uint64(n)
		}
		bw_mutex.Unlock()
	} else {
		n = -1
	}
	return
}

// SockWrite sends all the bytes, but respect the upload limit (force delays).
// Returns -1 and no error if we can't send any data now, because of bw limit.
func SockWrite(con net.Conn, buf []byte) (n int, e error) {
	var tosend int
	bw_mutex.Lock()
	ms := TickSent()
	if UploadLimit() == 0 {
		tosend = len(buf)
	} else {
		tosend = ms*int(UploadLimit())/1000 - ul_bytes_so_far
		if tosend > len(buf) {
			tosend = len(buf)
			if tosend > 4096 {
				tosend = 4096
			}
		} else if tosend < 0 {
			tosend = 0
		}
	}
	ul_bytes_so_far += tosend
	bw_mutex.Unlock()
	if tosend > 0 {
		// We used to have SetWriteDeadline() here, but it was causing problems because
		// in case of a timeout returned "n" was always 0, even if some data got sent.
		// see https://github.com/golang/go/issues/24727
		n, e = con.Write(buf[:tosend])
		bw_mutex.Lock()
		ul_bytes_so_far -= tosend
		if n > 0 {
			ul_bytes_so_far += n
			UlBytesTotal += uint64(n)
			ul_bytes_priod += uint64(n)
		}
		bw_mutex.Unlock()
	} else {
		n = -1
	}
	return
}

func LockBw() {
	bw_mutex.Lock()
}

func UnlockBw() {
	bw_mutex.Unlock()
}

func GetAvgBW(arr []uint64, idx uint16, cnt int) uint64 {
	var sum uint64
	if cnt <= 0 {
		return 0
	}
	for i := 0; i < cnt; i++ {
		idx--
		sum += arr[idx]
	}
	return sum / uint64(cnt)
}

func PrintBWStats() {
	bw_mutex.Lock()
	TickRecv()
	TickSent()
	fmt.Printf("Downloading at %d/%d KB/s, %s total",
		GetAvgBW(DlBytesPrevSec[:], DlBytesPrevSecIdx, 5)>>10, DownloadLimit()>>10, BytesToString(DlBytesTotal))
	fmt.Printf("  |  Uploading at %d/%d KB/s, %s total\n",
		GetAvgBW(UlBytesPrevSec[:], UlBytesPrevSecIdx, 5)>>10, UploadLimit()>>10, BytesToString(UlBytesTotal))
	bw_mutex.Unlock()
}

func SetDownloadLimit(val uint64) {
	atomic.StoreUint64(&download_limit, val)
}

func DownloadLimit() uint64 {
	return atomic.LoadUint64(&download_limit)
}

func SetUploadLimit(val uint64) {
	atomic.StoreUint64(&upload_limit, val)
}

func UploadLimit() (res uint64) {
	return atomic.LoadUint64(&upload_limit)
}

const (
	BwChartSlots     = 200 // number of records in the bandwidth chart
	BwChartMaxPeriod = 300 // BwChartSlots*BwChartMaxPeriod must be less than len(DlBytesPrevSec)
)

type BwChartDat struct {
	Avg [BwChartSlots]uint64 // average B/s within the slot. index 0 is the most recent slot
	Max [BwChartSlots]uint64 // peak B/s within the slot
}

type bwChartCache struct {
	dat     BwChartDat
	period  int64
	end_sec int64 // unix time where the current (still incomplete) slot begins
	valid   bool
}

var (
	dl_chart_cache bwChartCache
	ul_chart_cache bwChartCache
)

// update refreshes the cached chart records with time period aligned slots.
// The slot boundaries are at unix times divisible by the period, so records of
// completed slots never change and only the newly completed ones (usually none
// or one) need to be calculated. Make sure to call it with bw_mutex locked,
// just after TickRecv() / TickSent().
func (c *bwChartCache) update(arr []uint64, idx uint16, last_sec int64, period int64) {
	end := last_sec - last_sec%period // beginning of the current (incomplete) slot
	shift := BwChartSlots             // by default recalculate all the records...
	if c.valid && c.period == period && end >= c.end_sec {
		if end == c.end_sec {
			return // no new slot completed since the last call - the cache is up to date
		}
		if new_slots := (end - c.end_sec) / period; new_slots < BwChartSlots {
			shift = int(new_slots) // ... but usually only this many
		}
	}
	if shift < BwChartSlots {
		copy(c.dat.Avg[shift:], c.dat.Avg[:BwChartSlots-shift])
		copy(c.dat.Max[shift:], c.dat.Max[:BwChartSlots-shift])
	}
	// arr[idx-k] holds the byte count of the unix second last_sec-k (for k>=1)
	for i := shift - 1; i >= 0; i-- {
		var sum, max uint64
		slot_end := end - int64(i)*period
		for s := slot_end - period; s < slot_end; s++ {
			v := arr[idx-uint16(last_sec-s)]
			sum += v
			if v > max {
				max = v
			}
		}
		c.dat.Avg[i] = sum / uint64(period)
		c.dat.Max[i] = max
	}
	c.period = period
	c.end_sec = end
	c.valid = true
}

// GetBwChart fills the given records with the bandwidth chart data, where each
// record averages the given number of seconds (from 1 to BwChartMaxPeriod).
func GetBwChart(period int64, dl, ul *BwChartDat) {
	if period < 1 {
		period = 1
	} else if period > BwChartMaxPeriod {
		period = BwChartMaxPeriod
	}
	bw_mutex.Lock()
	TickRecv()
	TickSent()
	dl_chart_cache.update(DlBytesPrevSec[:], DlBytesPrevSecIdx, dl_last_sec, period)
	*dl = dl_chart_cache.dat
	ul_chart_cache.update(UlBytesPrevSec[:], UlBytesPrevSecIdx, ul_last_sec, period)
	*ul = ul_chart_cache.dat
	bw_mutex.Unlock()
}
