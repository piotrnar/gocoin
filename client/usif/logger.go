package usif

import (
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/piotrnar/gocoin/client/common"
)

func IPChecker(r *http.Request) bool {
	if common.NetworkClosed.Get() || Exit_now.Get() {
		return false
	}

	if r.TLS != nil { // for HTTPS mode we do not do IP checks
		r.ParseForm()
		return true
	}

	var a, b, c, d uint32
	n, _ := fmt.Sscanf(r.RemoteAddr, "%d.%d.%d.%d", &a, &b, &c, &d)
	if n != 4 {
		return false
	}
	addr := (a << 24) | (b << 16) | (c << 8) | d
	common.LockCfg()
	for i := range common.WebUIAllowed {
		if (addr & common.WebUIAllowed[i].Mask) == common.WebUIAllowed[i].Addr {
			common.UnlockCfg()
			r.ParseForm()
			return true
		}
	}
	common.UnlockCfg()
	println("ipchecker:", r.RemoteAddr, "is blocked")
	return false
}

// Request tracking structure
type IPStats struct {
	Count    int
	LastSeen time.Time
}

// Global tracker with mutex for thread safety
var (
	ipTracker = make(map[string]*IPStats)
	ipMutex   sync.RWMutex
)

func TrackingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Extract IP address (handle potential proxy headers)
		ss := strings.SplitN(r.RemoteAddr, ":", 2)
		ip := ss[0]

		// Update tracking info
		ipMutex.Lock()
		if stats, exists := ipTracker[ip]; exists {
			stats.Count++
			stats.LastSeen = time.Now()
		} else {
			ipTracker[ip] = &IPStats{
				Count:    1,
				LastSeen: time.Now(),
			}
		}
		ipMutex.Unlock()
		if IPChecker(r) {
			next.ServeHTTP(w, r)
		}
	})
}

// Function to get current stats (for displaying)
func GetWebUIStats() (res string) {
	ipMutex.RLock()
	for ip, stats := range ipTracker {
		res += fmt.Sprintf("%15s : %d requests, last %s ago\n", ip, stats.Count, time.Since(stats.LastSeen))
	}
	ipMutex.RUnlock()
	return
}
