package webui

import (
	"encoding/json"
	"net/http"

	"github.com/piotrnar/gocoin/client/common"
	"github.com/piotrnar/gocoin/client/usif"
	"github.com/piotrnar/gocoin/lib/btc"

	"strconv"
	"time"
)

func p_blocks(w http.ResponseWriter, r *http.Request) {
	if !ipchecker(r) {
		return
	}

	write_html_head(w, r)
	w.Write([]byte(load_template("blocks.html")))
	write_html_tail(w)
}

func json_blocks(w http.ResponseWriter, r *http.Request) {
	if !ipchecker(r) {
		return
	}

	type one_block struct {
		Height    uint32
		Timestamp uint32
		Hash      string
		TxCnt     int
		Size      int
		Weight    uint
		Version   uint32
		Reward    uint64
		Miner     string
		FeeSPB    float64

		Received                          uint32
		TimePre, TimeDl, TimeVer, TimeQue int
		WasteCnt                          uint
		MissedCnt                         int
		FromConID                         uint32
		Sigops                            int

		NonWitnessSize int

		HaveFeeStats bool

		PaidTxVSize uint
		TotalFees   uint64
	}

	var blks []*one_block

	common.Last.Mutex.Lock()
	end := common.Last.Block
	common.Last.Mutex.Unlock()

	for cnt := uint32(0); end != nil && cnt < common.GetUint32(&common.CFG.WebUI.ShowBlocks); cnt++ {
		bl, _, e := common.BlockChain.Blocks.BlockGet(end.BlockHash)
		if e != nil {
			break
		}
		block, e := btc.NewBlockX(bl, end.BlockHash)
		if e != nil {
			break
		}
		common.BlockChain.BlockIndexAccess.Lock()
		node := common.BlockChain.BlockIndex[end.BlockHash.BIdx()]
		common.BlockChain.BlockIndexAccess.Unlock()

		var cbasetx *btc.Tx

		rb, cbasetx := usif.GetReceivedBlockX(block)

		b := new(one_block)
		b.Height = end.Height
		b.Timestamp = block.BlockTime()
		b.Hash = end.BlockHash.String()
		b.TxCnt = block.TxCount
		b.Size = len(bl)
		b.Weight = rb.TheWeight
		b.NonWitnessSize = rb.NonWitnessSize
		b.Version = block.Version()

		for o := range cbasetx.TxOut {
			b.Reward += cbasetx.TxOut[o].Value
		}

		b.Miner, _ = common.TxMiner(cbasetx)
		b.PaidTxVSize = rb.ThePaidVSize
		b.TotalFees = b.Reward - btc.GetBlockReward(end.Height)
		if rb.ThePaidVSize > 0 {
			b.FeeSPB = float64(b.TotalFees) / float64(rb.ThePaidVSize)
		}

		b.Received = uint32(rb.TmStart.Unix())
		b.Sigops = int(node.SigopsCost)

		if rb.TmPreproc.IsZero() {
			b.TimePre = -1
		} else {
			b.TimePre = int(rb.TmPreproc.Sub(rb.TmStart) / time.Millisecond)
		}

		if rb.TmDownload.IsZero() {
			b.TimeDl = -1
		} else {
			b.TimeDl = int(rb.TmDownload.Sub(rb.TmStart) / time.Millisecond)
		}

		if rb.TmQueue.IsZero() {
			b.TimeQue = -1
		} else {
			b.TimeQue = int(rb.TmQueue.Sub(rb.TmStart) / time.Millisecond)
		}

		if rb.TmAccepted.IsZero() {
			b.TimeVer = -1
		} else {
			b.TimeVer = int(rb.TmAccepted.Sub(rb.TmStart) / time.Millisecond)
		}

		b.WasteCnt = rb.Cnt
		b.MissedCnt = rb.TxMissing
		b.FromConID = rb.FromConID

		usif.BlockFeesMutex.Lock()
		_, b.HaveFeeStats = usif.BlockFees[end.Height]
		usif.BlockFeesMutex.Unlock()

		blks = append(blks, b)
		end = end.Parent
	}

	bx, er := json.Marshal(blks)
	if er == nil {
		w.Header()["Content-Type"] = []string{"application/json"}
		w.Write(bx)
	} else {
		println(er.Error())
	}

}

func json_blfees(w http.ResponseWriter, r *http.Request) {
	if !ipchecker(r) {
		return
	}

	if len(r.Form["height"]) == 0 {
		w.Write([]byte("No hash given"))
		return
	}

	height, e := strconv.ParseUint(r.Form["height"][0], 10, 32)
	if e != nil {
		w.Write([]byte(e.Error()))
		return
	}

	usif.BlockFeesMutex.Lock()
	fees, ok := usif.BlockFees[uint32(height)]
	usif.BlockFeesMutex.Unlock()

	if !ok {
		w.Write([]byte("File not found"))
		return
	}

	bx, er := json.Marshal(fees)
	if er == nil {
		w.Header()["Content-Type"] = []string{"application/json"}
		w.Write(bx)
	} else {
		println(er.Error())
	}
}
