package main

import (
	"fmt"
	"time"
	"sort"
	"strings"
	"net/http"
	"github.com/piotrnar/gocoin/btc"
)

type onemiernstat []struct {
	name string
	cnt int
}

func (x onemiernstat) Len() int {
	return len(x)
}

func (x onemiernstat) Less(i, j int) bool {
	if x[i].cnt == x[j].cnt {
		return x[i].name < x[j].name // Same numbers: sort by name
	}
	return x[i].cnt > x[j].cnt
}

func (x onemiernstat) Swap(i, j int) {
	x[i], x[j] = x[j], x[i]
}

func p_miners(w http.ResponseWriter, r *http.Request) {
	m := make(map[string]int, 20)
	cnt, unkn := 0, 0
	end := BlockChain.BlockTreeEnd
	var lastts int64
	var diff float64
	now := time.Now().Unix()
	for ; end!=nil; cnt++ {
		if now-int64(end.Timestamp) > 24*3600 {
			break
		}
		lastts = int64(end.Timestamp)
		bl, _, e := BlockChain.Blocks.BlockGet(end.BlockHash)
		if e != nil {
			return
		}
		diff += btc.GetDifficulty(end.Bits)
		miner := blocks_miner(bl)
		if miner!="" {
			m[miner]++
		} else {
			unkn++
		}
		end = end.Parent
	}
	srt := make(onemiernstat, len(m))
	i := 0
	for k, v := range m {
		srt[i].name = k
		srt[i].cnt = v
		i++
	}
	sort.Sort(srt)

	mnrs := load_template("miners.html")
	onerow := load_template("miners_row.html")

	diff /= float64(cnt)
	bph := float64(cnt)/24
	hrate := bph/6 * diff * 7158278.826667
	mnrs = strings.Replace(mnrs, "{BLOCKS_COUNT}", fmt.Sprint(cnt), 1)
	mnrs = strings.Replace(mnrs, "{FIRST_BLOCK_TIME}", time.Unix(lastts, 0).Format("2006-01-02 15:04:05"), 1)
	mnrs = strings.Replace(mnrs, "{AVG_BLOCKS_PER_HOUR}", fmt.Sprintf("%.2f", bph), 1)
	mnrs = strings.Replace(mnrs, "{AVG_DIFFICULTY}", fmt.Sprintf("%.2f", diff), 1)
	mnrs = strings.Replace(mnrs, "{AVG_HASHDATE}", hr2str(hrate), 1)


	for i := range srt {
		s := onerow
		s = strings.Replace(s, "{MINER_NAME}", srt[i].name, 1)
		s = strings.Replace(s, "{BLOCK_COUNT}", fmt.Sprint(srt[i].cnt), 1)
		s = strings.Replace(s, "{TOTAL_PERCENT}", fmt.Sprintf("%.0f", 100*float64(srt[i].cnt)/float64(cnt)), 1)
		s = strings.Replace(s, "{MINER_HASHRATE}", hr2str(hrate*float64(srt[i].cnt)/float64(cnt)), 1)
		mnrs = strings.Replace(mnrs, "{MINER_ROW}", s+"{MINER_ROW}", 1)
	}

	onerow = strings.Replace(onerow, "{MINER_NAME}", "<i>Unknown</i>", 1)
	onerow = strings.Replace(onerow, "{BLOCK_COUNT}", fmt.Sprint(unkn), 1)
	onerow = strings.Replace(onerow, "{TOTAL_PERCENT}", fmt.Sprintf("%.0f", 100*float64(unkn)/float64(cnt)), 1)
	onerow = strings.Replace(onerow, "{MINER_HASHRATE}", hr2str(hrate*float64(unkn)/float64(cnt)), 1)
	mnrs = strings.Replace(mnrs, "{MINER_ROW}", onerow, 1)

	write_html_head(w, r)
	w.Write([]byte(mnrs))
	write_html_tail(w)
}
