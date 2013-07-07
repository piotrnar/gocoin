package main

import (
	"fmt"
	"time"
	"strings"
	"net/http"
	"github.com/piotrnar/gocoin/btc"
)

func p_blocks(w http.ResponseWriter, r *http.Request) {
	blks := load_template("blocks.html")
	onerow := load_template("blocks_row.html")

	end := BlockChain.BlockTreeEnd
	for cnt:=0; end!=nil && cnt<100; cnt++ {
		bl, _, e := BlockChain.Blocks.BlockGet(end.BlockHash)
		if e != nil {
			return
		}
		block, e := btc.NewBlock(bl)
		if e!=nil {
			return
		}
		block.BuildTxList()
		s := onerow

		s = strings.Replace(s, "{BLOCK_NUMBER}", fmt.Sprint(end.Height), 1)
		s = strings.Replace(s, "{BLOCK_TIMESTAMP}",
			time.Unix(int64(block.BlockTime), 0).Format("Mon 15:04:05"), 1)
		s = strings.Replace(s, "{BLOCK_HASH}", end.BlockHash.String(), 1)
		s = strings.Replace(s, "{BLOCK_TXS}", fmt.Sprint(len(block.Txs)), 1)
		s = strings.Replace(s, "{BLOCK_SIZE}", fmt.Sprintf("%.1f", float64(len(bl))/1000), 1)
		var rew uint64
		for o := range block.Txs[0].TxOut {
			rew += block.Txs[0].TxOut[o].Value
		}
		s = strings.Replace(s, "{BLOCK_REWARD}", fmt.Sprintf("%.2f", float64(rew)/1e8), 1)
		s = strings.Replace(s, "{BLOCK_MINER}", blocks_miner(bl), 1)

		rb := receivedBlocks[end.BlockHash.BIdx()]
		if rb.tmDownload!=0 {
			s = strings.Replace(s, "{TIME_TO_DOWNLOAD}", fmt.Sprint(int(rb.tmDownload/time.Millisecond)), 1)
		} else {
			s = strings.Replace(s, "{TIME_TO_DOWNLOAD}", "", 1)
		}
		if rb.tmAccept!=0 {
			s = strings.Replace(s, "{TIME_TO_ACCEPT}", fmt.Sprint(int(rb.tmAccept/time.Millisecond)), 1)
		} else {
			s = strings.Replace(s, "{TIME_TO_ACCEPT}", "", 1)
		}
		if rb.cnt!=0 {
			s = strings.Replace(s, "{WASTED_BLOCKS}", fmt.Sprint(rb.cnt), 1)
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
