package main

import (
	"fmt"
	"time"
	"bytes"
	"strconv"
	"github.com/piotrnar/gocoin/btc"
)

var MinerIds = [][2]string{
	{"BTC Guild", "BTC Guild"},
	{"ASICMiner", "ASICMiner"},
	{"50BTC", "50BTC.com"},
	{"Slush", "/slush/"},
	// Dont know how to do Deepbit
	{"EclipseMC", "EMC "},
	{"Eligius", "Eligius"},
	{"BitMinter", "BitMinter"},
	{"Bitparking", "bitparking"},
	{"CoinLab", "CoinLab"},
	{"Triplemining", "Triplemining.com"},
	{"Ozcoin", "ozcoin"},
	{"SatoshiSys", "Satoshi Systems"},
	{"ST Mining", "st mining corp"},
}


func mined_by(bl []byte, id string) bool {
	max2search := 0x200
	if len(bl)<max2search {
		max2search = len(bl)
	}
	return bytes.Index(bl[0x51:max2search], []byte(id))!=-1
}


func mined_by_us(bl []byte) bool {
	mutex_cfg.Lock()
	minid := CFG.Beeps.MinerID
	mutex_cfg.Unlock()
	if minid=="" {
		return false
	}
	return mined_by(bl, minid)
}


func blocks_miner(bl []byte) (string, int) {
	for i := range MinerIds {
		if mined_by(bl, MinerIds[i][1]) {
			return MinerIds[i][0], i
		}
	}
	return "", -1
}


func hr2str(hr float64) string {
	if hr>1e12 {
		return fmt.Sprintf("%.2f TH/s", hr/1e12)
	}
	if hr>1e9 {
		return fmt.Sprintf("%.2f GH/s", hr/1e9)
	}
	return fmt.Sprintf("%.2f MH/s", hr/1e6)
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
	Last.mutex.Lock()
	bte := Last.Block
	end := bte
	Last.mutex.Unlock()
	cnt, diff := 0, float64(0)
	tot_blocks, tot_blocks_len := 0, 0
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
		tot_blocks_len += len(bl)
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
	mutex_cfg.Lock()
	if CFG.Beeps.MinerID!="" {
		fmt.Printf("%.8f BTC mined by %s, in %d blocks for the last %d hours\n",
			float64(totbtc)/1e8, CFG.Beeps.MinerID, cnt, hrs)
	}
	mutex_cfg.Unlock()
	if cnt > 0 {
		fmt.Printf("Projected weekly income : %.0f BTC,  estimated hashrate : %s\n",
			7*24*float64(totbtc)/float64(hrs)/1e8,
			hr2str(float64(cnt)/float64(6*hrs) * diff * 7158278.826667))
	}
	bph := float64(tot_blocks)/float64(hrs)
	fmt.Printf("Total network hashrate : %s @ average diff %.0f  (%.2f bph)\n",
		hr2str(bph/6 * diff * 7158278.826667), diff, bph)
	fmt.Printf("Average block size was %.1f KB,  next difficulty change in %d blocks\n",
		float64(tot_blocks_len/tot_blocks)/1e3, 2016-bte.Height%2016)
}


func set_miner(p string) {
	if p=="" {
		fmt.Println("Specify MinerID string or one of the numberic values:")
		for i := range MinerIds {
			fmt.Printf("%3d - %s\n", i, MinerIds[i][0])
		}
		return
	}

	if p=="off" {
		mutex_cfg.Lock()
		CFG.Beeps.MinerID = ""
		mutex_cfg.Unlock()
		fmt.Printf("Mining monitor disabled\n")
		return
	}

	v, e := strconv.ParseUint(p, 10, 32)
	mutex_cfg.Lock()
	if e!=nil {
		CFG.Beeps.MinerID = p
	} else if int(v)<len(MinerIds) {
		CFG.Beeps.MinerID = MinerIds[v][1]
	} else {
		fmt.Println("The number is too big. Max is", len(MinerIds)-1)
	}
	fmt.Printf("Current miner ID: '%s'\n", CFG.Beeps.MinerID)
	mutex_cfg.Unlock()
}


func init() {
	newUi("minerset mid", false, set_miner, "Setup the mining monitor with the given ID, or off to disable the monitor")
	newUi("minerstat m", false, do_mining, "Look for the miner ID in recent blocks (optionally specify number of hours)")
}
