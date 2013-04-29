package main                   

import (
	"sync"
	"time"
	"fmt"
)

const secondsBack = 60


var (
	bw_mutex sync.Mutex

	tot_up, tot_dn uint64

	siz_sent map[uint32]uint64 = make(map[uint32]uint64, secondsBack)
	sec_sent uint32

	siz_rcvd map[uint32]uint64 = make(map[uint32]uint64, secondsBack)
	sec_rcvd uint32
)


func bw_sent(siz int) {
	bw_mutex.Lock()
	tot_up += uint64(siz)
	now := uint32(time.Now().Unix())
	if cv, ok := siz_sent[now]; ok {
		siz_sent[secondsBack-1] = cv+uint64(siz)
	} else {
		siz_sent[secondsBack-1] = uint64(siz)
		for k, _ := range siz_sent {
			if k<now-secondsBack {
				delete(siz_sent, k)
			}
		}
	}
	bw_mutex.Unlock()
}

func bw_got(siz int) {
	bw_mutex.Lock()
	tot_dn += uint64(siz)
	now := uint32(time.Now().Unix())
	if cv, ok := siz_rcvd[now]; ok {
		siz_rcvd[now] = cv+uint64(siz)
	} else {
		siz_rcvd[now] = uint64(siz)
		for k, _ := range siz_rcvd {
			if k<now-secondsBack {
				delete(siz_rcvd, k)
			}
		}
	}
	bw_mutex.Unlock()
}


func bw_stats() (s string) {
	bw_mutex.Lock()
	
	now := uint32(time.Now().Unix())
	sum := uint64(0)
	sum5 := uint64(0)
	for i := now-secondsBack; i<=now; i++ {
		if v, ok := siz_rcvd[i]; ok {
			sum += v
			if (now-i)<5 {
				sum5 += v
			}
		}
	}
	s += fmt.Sprintf("DOWN:[%d/%d kbps,  %d MB tot]  ",
		(sum/secondsBack)>>7, (sum5/5)>>7, tot_dn>>20)
	
	sum, sum5 = 0, 0
	for i := now-secondsBack; i<=now; i++ {
		if v, ok := siz_sent[i]; ok {
			sum += v
			if (now-i)<5 {
				sum5 += v
			}
		}
	}
	s += fmt.Sprintf(" UP:[%d/%d kbps, %d MB tot]",
		(sum/secondsBack)>>7, (sum5/5)>>7, tot_up>>20)

	bw_mutex.Unlock()
	return
}
