package textui

import (
	"fmt"
	"regexp"
	"strconv"
	"time"

	"github.com/piotrnar/gocoin/client/common"
	"github.com/piotrnar/gocoin/client/rpcapi"
	"github.com/piotrnar/gocoin/lib/btc"
)

func do_mining(s string) {
	var totbtc, hrs, segwit_cnt uint64
	if s != "" {
		hrs, _ = strconv.ParseUint(s, 10, 64)
	}
	if hrs == 0 {
		hrs = uint64(common.CFG.Stat.MiningHrs)
	}
	fmt.Println("Looking back", hrs, "hours...")
	lim := uint32(time.Now().Add(-time.Hour * time.Duration(hrs)).Unix())
	common.Last.Mutex.Lock()
	bte := common.Last.Block
	end := bte
	common.Last.Mutex.Unlock()
	cnt, diff := 0, float64(0)
	tot_blocks, tot_blocks_len := 0, 0

	bip100_voting := make(map[string]uint)
	bip100x := regexp.MustCompile("/BV{0,1}[0-9]+[M]{0,1}/")

	eb_ad_voting := make(map[string]uint)
	eb_ad_x := regexp.MustCompile("/EB[0-9]+/AD[0-9]+/")

	for end.Timestamp() >= lim {
		bl, _, e := common.BlockChain.Blocks.BlockGet(end.BlockHash)
		if e != nil {
			println(cnt, e.Error())
			return
		}
		block, e := btc.NewBlock(bl)
		if e != nil {
			println("btc.NewBlock failed", e.Error())
			return
		}

		bt, _ := btc.NewBlock(bl)
		cbasetx, _ := btc.NewTx(bl[bt.TxOffset:])

		tot_blocks++
		tot_blocks_len += len(bl)
		diff += btc.GetDifficulty(block.Bits())

		if (block.Version() & 0x20000002) == 0x20000002 {
			segwit_cnt++
		}

		res := bip100x.Find(cbasetx.TxIn[0].ScriptSig)
		if res != nil {
			bip100_voting[string(res)]++
			nimer, _ := common.TxMiner(cbasetx)
			fmt.Println("      block", end.Height, "by", nimer, "BIP100 voting", string(res), " total:", bip100_voting[string(res)])
		}

		res = eb_ad_x.Find(cbasetx.TxIn[0].ScriptSig)
		if res != nil {
			eb_ad_voting[string(res)]++
		}

		end = end.Parent
	}
	if tot_blocks == 0 {
		fmt.Println("There are no blocks from the last", hrs, "hour(s)")
		return
	}
	diff /= float64(tot_blocks)
	if cnt > 0 {
		fmt.Printf("Projected weekly income : %.0f BTC,  estimated hashrate : %s\n",
			7*24*float64(totbtc)/float64(hrs)/1e8,
			common.HashrateToString(float64(cnt)/float64(6*hrs)*diff*7158278.826667))
	}
	bph := float64(tot_blocks) / float64(hrs)
	fmt.Printf("Total network hashrate : %s @ average diff %.0f  (%.2f bph)\n",
		common.HashrateToString(bph/6*diff*7158278.826667), diff, bph)
	fmt.Printf("%d blocks in %d hours. Average size %.1f KB,  next diff in %d blocks\n",
		tot_blocks, hrs, float64(tot_blocks_len/tot_blocks)/1e3, 2016-bte.Height%2016)

	fmt.Printf("\nSegWit Voting: %d (%.1f%%)\n", segwit_cnt, float64(segwit_cnt)*100/float64(tot_blocks))
	fmt.Println()
	fmt.Println("BU Voting")
	for k, v := range eb_ad_voting {
		fmt.Printf(" %s \t %d \t %.1f%%\n", k, v, float64(v)*100/float64(tot_blocks))
	}
}

func do_segwit(s string) {
	rpcapi.DO_SEGWIT = s == "1"
	fmt.Println("DO_SEGWIT:", rpcapi.DO_SEGWIT)
}

func do_minon(s string) {
	rpcapi.DO_NOT_SUBMIT = s == "1"
	fmt.Println("DO_NOT_SUBMIT:", rpcapi.DO_NOT_SUBMIT)
}

func do_minsec(s string) {
	val, er := strconv.ParseInt(s, 10, 32)
	if er == nil {
		rpcapi.WAIT_FOR_SECONDS = int(val) * 60
	}
	fmt.Println("WAIT_FOR_SECONDS:", rpcapi.WAIT_FOR_SECONDS, "  -> ", rpcapi.WAIT_FOR_SECONDS/60, "minutes")
}

func do_minaddr(s string) {
	if s != "" {
		if _, e := btc.NewAddrFromString(s); e == nil {
			rpcapi.COINBASE_ADDRESS = s
		} else {
			println(e.Error())
		}
	}
	fmt.Println("COINBASE_ADDRESS:", rpcapi.COINBASE_ADDRESS)
}

func do_minstring(s string) {
	if s != "" {
		if len(s) > 200 {
			rpcapi.COINBASE_STRING = s[:200]
		} else {
			rpcapi.COINBASE_STRING = s
		}
	}
	fmt.Println("COINBASE_STRING:", rpcapi.COINBASE_STRING)
}

func init() {
	newUi("minerstat m", false, do_mining, "Look for the miner ID in recent blocks (optionally specify number of hours)")
	newUi("minsw", false, do_segwit, "Mine segwit blocks (0 or 1)")
	newUi("minstop ms", false, do_minon, "Tur off submtting mined blocks (0 or 1)")
	newUi("minminutes mm", false, do_minsec, "Allow forcing of difficulty one in so many mintes after (before, if negative) last block's timestamp")
	newUi("minaddr ma", false, do_minaddr, "Set the address to be put inside the coinbase")
	newUi("minstring mx", false, do_minstring, "Specify the string to be put inside the coinbase")
}
