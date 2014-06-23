package webui

import (
	"fmt"
	"time"
	"sort"
	"strings"
	"net/http"
	"github.com/piotrnar/gocoin/lib/btc"
	"github.com/piotrnar/gocoin/client/common"
)

type omv struct {
	cnt int
	bts uint64
	mid int
}

type onemiernstat []struct {
	name string
	omv
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
	if !ipchecker(r) {
		return
	}

	common.ReloadMiners()

	m := make(map[string] omv, 20)
	var om omv
	cnt := 0
	common.Last.Mutex.Lock()
	end := common.Last.Block
	common.Last.Mutex.Unlock()
	var lastts int64
	var diff float64
	var totbts uint64
	current_mid := -1
	now := time.Now().Unix()

	common.LockCfg()
	minerid := common.CFG.Beeps.MinerID
	common.UnlockCfg()

	next_diff_change := 2016-end.Height%2016

	for ; end!=nil; cnt++ {
		if now-int64(end.Timestamp()) > int64(common.CFG.MiningStatHours)*3600 {
			break
		}
		lastts = int64(end.Timestamp())
		bl, _, e := common.BlockChain.Blocks.BlockGet(end.BlockHash)
		if e != nil {
			return
		}
		diff += btc.GetDifficulty(end.Bits())
		miner, mid := common.BlocksMiner(bl)
		if mid==-1 {
			miner = "<i>" + miner + "</i>"
		}
		om = m[miner]
		om.cnt++
		om.bts+= uint64(len(bl))
		om.mid = mid
		m[miner] = om
		if mid!=-1 && current_mid==-1 && minerid==string(common.MinerIds[mid].Tag) {
			current_mid = mid
		}
		totbts += uint64(len(bl))
		end = end.Parent
	}

	if cnt==0 {
		write_html_head(w, r)
		w.Write([]byte(fmt.Sprint("No blocks in last ", common.CFG.MiningStatHours, " hours")))
		write_html_tail(w)
		return
	}

	srt := make(onemiernstat, len(m))
	i := 0
	for k, v := range m {
		srt[i].name = k
		srt[i].omv = v
		i++
	}
	sort.Sort(srt)

	mnrs := load_template("miners.html")
	onerow := load_template("miners_row.html")

	diff /= float64(cnt)
	bph := float64(cnt)/float64(common.CFG.MiningStatHours)
	hrate := bph/6 * diff * 7158278.826667
	mnrs = strings.Replace(mnrs, "{MINING_HOURS}", fmt.Sprint(common.CFG.MiningStatHours), 1)
	mnrs = strings.Replace(mnrs, "{BLOCKS_COUNT}", fmt.Sprint(cnt), 1)
	mnrs = strings.Replace(mnrs, "{FIRST_BLOCK_TIME}", time.Unix(lastts, 0).Format("2006-01-02 15:04:05"), 1)
	mnrs = strings.Replace(mnrs, "{AVG_BLOCKS_PER_HOUR}", fmt.Sprintf("%.2f", bph), 1)
	mnrs = strings.Replace(mnrs, "{AVG_DIFFICULTY}", common.NumberToString(diff), 1)
	mnrs = strings.Replace(mnrs, "{AVG_HASHDATE}", common.HashrateToString(hrate), 1)
	mnrs = strings.Replace(mnrs, "{AVG_BLOCKSIZE}", fmt.Sprintf("%.1fKB", float64(totbts)/float64(cnt)/1000), 1)
	mnrs = strings.Replace(mnrs, "{DIFF_CHANGE_IN}", fmt.Sprint(next_diff_change), 1)
	mnrs = strings.Replace(mnrs, "{MINER_MON_IDX}", fmt.Sprint(current_mid), 1)

	for i := range srt {
		s := onerow
		s = strings.Replace(s, "{MINER_NAME}", srt[i].name, 1)
		s = strings.Replace(s, "{BLOCK_COUNT}", fmt.Sprint(srt[i].cnt), 1)
		s = strings.Replace(s, "{TOTAL_PERCENT}", fmt.Sprintf("%.0f", 100*float64(srt[i].cnt)/float64(cnt)), 1)
		s = strings.Replace(s, "{MINER_HASHRATE}", common.HashrateToString(hrate*float64(srt[i].cnt)/float64(cnt)), 1)
		s = strings.Replace(s, "{AVG_BLOCK_SIZE}", fmt.Sprintf("%.1fKB", float64(srt[i].bts)/float64(srt[i].cnt)/1000), 1)
		s = strings.Replace(s, "{MINER_ID}", fmt.Sprint(srt[i].mid), -1)
		mnrs = templ_add(mnrs, "<!--MINER_ROW-->", s)
	}

	write_html_head(w, r)
	w.Write([]byte(mnrs))
	write_html_tail(w)
}
