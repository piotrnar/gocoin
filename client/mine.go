package main

import (
	"fmt"
	"time"
	"bytes"
	"strconv"
	"github.com/piotrnar/gocoin/btc"
)

var minerId string = "Mined By ASICMiner"


func mined_by_us(bl []byte) bool {
	max2search := 0x200
	if len(bl)<max2search {
		max2search = len(bl)
	}
	return bytes.Index(bl[0x51:max2search], []byte(minerId))!=-1
}


func do_mining(s string) {
	var totbtc, hrs uint64
	if s != "" {
		hrs, _ = strconv.ParseUint(s, 10, 64)
	}
	if hrs == 0 {
		hrs = 24
	}
	fmt.Println("Looking back", hrs, "hours...")
	lim := uint32(time.Now().Add(-time.Hour*time.Duration(hrs)).Unix())
	end := BlockChain.BlockTreeEnd
	cnt, diff := 0, float64(0)
	tot_blocks := 0
	for end.Timestamp >= lim {
		bl, _, e := BlockChain.Blocks.BlockGet(end.BlockHash)
		if e != nil {
			println(cnt, e.Error())
			return
		}
		block, e := btc.NewBlock(bl)
		if e!=nil {
			println("btc.NewBlock failed", e.Error())
			return
		}
		tot_blocks++
		diff += btc.GetDifficulty(block.Bits)
		if mined_by_us(bl) {
			block.BuildTxList()
			totbtc += block.Txs[0].TxOut[0].Value
			cnt++
			fmt.Printf("%4d) %6d %s %s  %5.2f => %5.2f BTC total, %d txs, %.1f KB\n",
				cnt, end.Height, end.BlockHash.String(),
				time.Unix(int64(end.Timestamp), 0).Format("2006-01-02 15:04:05"),
				float64(block.Txs[0].TxOut[0].Value)/1e8, float64(totbtc)/1e8,
				len(block.Txs), float64(len(bl))/1e3)
		}
		end = end.Parent
	}
	if tot_blocks == 0 {
		fmt.Println("There are no blocks from the last", hrs, "hour(s)")
		return
	}
	diff /= float64(tot_blocks)
	fmt.Printf("%.8f BTC mined in %d blocks for the last %d hours\n",
		float64(totbtc)/1e8, cnt, hrs)
	if cnt > 0 {
		fmt.Printf("Projected weekly income : %.0f BTC,  estimated hashrate : %.2f TH/s\n",
			7*24*float64(totbtc)/float64(hrs)/1e8,
			float64(cnt)/float64(6*hrs) * diff * 7158278.826667 / 1e12)
	}
	bph := float64(tot_blocks)/float64(hrs)
	fmt.Printf("Total network hashrate : %.2f TH/s @ average diff %.0f  (%.2f bph)\n",
		bph/6 * diff * 7158278.826667 / 1e12, diff, bph)
}


func set_miner(p string) {
	if len(p)>3 {
		minerId = p
	}
	fmt.Printf("Current miner ID: '%s'\n", minerId)
}


func init() {
	newUi("minerset mid", false, set_miner, "Set the miner ID (specify a string paremeter)")
	newUi("minerstat m", false, do_mining, "Look for the miner ID in recent blocks")
}
