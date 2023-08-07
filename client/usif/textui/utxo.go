package textui

import (
	"fmt"
	"math"
	"sort"

	"github.com/piotrnar/gocoin/client/common"
	"github.com/piotrnar/gocoin/lib/utxo"
)

// inspired by https://bitcoinmagazine.com/technical/utxoracle-model-could-bring-use-cases-to-bitcoin
func wallet_usd(s string) {
	const MIN_BTC_VALUE = 20e3 // 0.001 BTC (not included)
	const MAX_BTC_VALUE = 1e6  // 0.01 BTC (not included)
	const VALUE_MULTIPLIER = 1000.0

	var blocks_back int
	var most_often_sent_usd_amount float64
	blocks_back = 144
	most_often_sent_usd_amount = 50.0
	fmt.Sscanf(s, "%d %f", &blocks_back, &most_often_sent_usd_amount)

	if blocks_back < 1 {
		println("Only positiove block values are allowed")
		return
	}
	if most_often_sent_usd_amount < 10 || most_often_sent_usd_amount > 2000 {
		println("Use USD amount between 10 and 2000")
		return
	}

	db := common.BlockChain.Unspent

	min_index := int64(VALUE_MULTIPLIER * (math.Log10(MIN_BTC_VALUE)))
	max_index := int64(VALUE_MULTIPLIER * (math.Log10(MAX_BTC_VALUE)))

	fmt.Printf("Checking UTXO from the last %d blocks, assuming the most common amount was %.2f USD\n", blocks_back, most_often_sent_usd_amount)

	var rec *utxo.UtxoRec
	from_block := db.LastBlockHeight - uint32(blocks_back)
	occ := make([]int64, max_index-min_index)
	for i := range db.HashMap {
		for k, v := range db.HashMap[i] {
			rec = utxo.NewUtxoRecStatic(k, v)
			if rec.InBlock < from_block {
				continue
			}
			for _, o := range rec.Outs {
				if o != nil && o.Value > MIN_BTC_VALUE && o.Value < MAX_BTC_VALUE {
					v10 := int64(VALUE_MULTIPLIER * (math.Log10(float64(o.Value))))
					occ[v10-min_index]++
				}
			}
		}
	}

	srtd := make([][2]int, len(occ)-2)
	var ccc int
	for i := 1; i < len(occ)-1; i++ {
		val := occ[i-1] + occ[i] + occ[i+1]
		srtd[ccc][0] = i
		srtd[ccc][1] = int(val)
		ccc++
	}
	sort.Slice(srtd, func(i, j int) bool { return srtd[i][1] > srtd[j][1] })
	fmt.Println("Most commonly used amounts:")
	for i := range srtd[:10] {
		hhi := srtd[i][0] + int(min_index)
		hhval := srtd[i][1]
		xxx := math.Pow(10, float64(hhi)/VALUE_MULTIPLIER)
		fmt.Printf("Found high count (%d) at %d (%d sats). Estimated BTC price: %d USD\n",
			hhval, hhi, int(xxx), int(most_often_sent_usd_amount*1e8/xxx))
	}

}

func init() {
	newUi("usd", true, wallet_usd, "Try to figure out recent BTC/USD price [block_count [most_common_usd]]")
}
