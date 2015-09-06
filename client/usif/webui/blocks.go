package webui

import (
	"fmt"
	"time"
	"strings"
	"net/http"
	"sync/atomic"
	"github.com/piotrnar/gocoin/lib/btc"
	"github.com/piotrnar/gocoin/client/common"
	"github.com/piotrnar/gocoin/client/network"
)

func p_blocks(w http.ResponseWriter, r *http.Request) {
	if !ipchecker(r) {
		return
	}

	blks := load_template("blocks.html")
	onerow := load_template("blocks_row.html")

	common.Last.Mutex.Lock()
	end := common.Last.Block
	common.Last.Mutex.Unlock()

	for cnt:=uint32(0); end!=nil && cnt<atomic.LoadUint32(&common.CFG.WebUI.ShowBlocks); cnt++ {
		bl, _, e := common.BlockChain.Blocks.BlockGet(end.BlockHash)
		if e != nil {
			return
		}
		block, e := btc.NewBlock(bl)
		if e!=nil {
			return
		}
		cbasetx, _ := btc.NewTx(bl[block.TxOffset:])
		s := onerow

		s = strings.Replace(s, "{BLOCK_NUMBER}", fmt.Sprint(end.Height), 1)
		s = strings.Replace(s, "{BLOCK_TIMESTAMP}",
			time.Unix(int64(block.BlockTime()), 0).Format("Mon 15:04:05"), 1)
		s = strings.Replace(s, "{BLOCK_HASH}", end.BlockHash.String(), 1)
		s = strings.Replace(s, "{BLOCK_TXS}", fmt.Sprint(block.TxCount), 1)
		s = strings.Replace(s, "{BLOCK_SIZE}", fmt.Sprintf("%.1f", float64(len(bl))/1000), 1)
		var rew uint64
		for o := range cbasetx.TxOut {
			rew += cbasetx.TxOut[o].Value
		}
		s = strings.Replace(s, "<!--BLOCK_VERSION-->", fmt.Sprint(block.Version()), 1)
		s = strings.Replace(s, "{BLOCK_REWARD}", fmt.Sprintf("%.2f", float64(rew)/1e8), 1)
		mi, _ := common.TxMiner(cbasetx)
		if len(mi)>10 {
			mi = mi[:10]
		}
		s = strings.Replace(s, "{BLOCK_MINER}", mi, 1)


		network.MutexRcv.Lock()
		rb := network.ReceivedBlocks[end.BlockHash.BIdx()]
		network.MutexRcv.Unlock()

		if rb.TmDownload!=0 {
			s = strings.Replace(s, "{TIME_TO_DOWNLOAD}", fmt.Sprint(int(rb.TmDownload/time.Millisecond)), 1)
		} else {
			s = strings.Replace(s, "{TIME_TO_DOWNLOAD}", "", 1)
		}
		if rb.TmAccept!=0 {
			s = strings.Replace(s, "{TIME_TO_ACCEPT}", fmt.Sprint(int(rb.TmAccept/time.Millisecond)), 1)
		} else {
			s = strings.Replace(s, "{TIME_TO_ACCEPT}", "", 1)
		}
		if rb.Cnt!=0 {
			s = strings.Replace(s, "{WASTED_BLOCKS}", fmt.Sprint(rb.Cnt), 1)
		} else {
			s = strings.Replace(s, "{WASTED_BLOCKS}", "", 1)
		}

		blks = templ_add(blks, "<!--BLOCK_ROW-->", s)

		end = end.Parent
	}

	write_html_head(w, r)
	w.Write([]byte(blks))
	write_html_tail(w)
}
