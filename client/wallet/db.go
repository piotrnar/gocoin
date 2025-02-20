package wallet

import (
	"encoding/binary"
	"fmt"
	"slices"
	"sync"

	"github.com/piotrnar/gocoin/client/common"
	"github.com/piotrnar/gocoin/lib/btc"
	"github.com/piotrnar/gocoin/lib/script"
	"github.com/piotrnar/gocoin/lib/utxo"
)

const (
	IDX_P2KH  = 0
	IDX_P2SH  = 1
	IDX_P2WKH = 2
	IDX_P2WSH = 3
	IDX_P2TAP = 4
	IDX_CNT   = 5
)

var (
	AllBalances [IDX_CNT]map[string]*OneAllAddrBal
	AccessMutex sync.Mutex
	IDX2SYMB    [IDX_CNT]string = [IDX_CNT]string{"P2KH", "P2SH", "P2WKH", "P2WSH", "P2TAP"}
	IDX2SIZE    [IDX_CNT]int    = [IDX_CNT]int{20, 20, 20, 32, 32}
)

type OneAllAddrInp [utxo.UtxoIdxLen + 4]byte

type OneAllAddrBal struct {
	Value   uint64 // Highest bit of it means P2SH
	unsp    []OneAllAddrInp
	unspMap map[OneAllAddrInp]bool
}

func (ur *OneAllAddrInp) GetRec() (rec *utxo.UtxoRec, vout uint32) {
	var ind utxo.UtxoKeyType
	copy(ind[:], ur[:])
	common.BlockChain.Unspent.MapMutex[int(ind[0])].RLock()
	v := common.BlockChain.Unspent.HashMap[int(ind[0])][ind]
	common.BlockChain.Unspent.MapMutex[int(ind[0])].RUnlock()
	if v != nil {
		vout = binary.LittleEndian.Uint32(ur[utxo.UtxoIdxLen:])
		rec = utxo.NewUtxoRec(ind, v)
	}
	return
}

func Script2Idx(pkscr []byte) (idx int, uidx []byte) {
	if script.IsP2KH(pkscr) {
		uidx = pkscr[3:23]
		idx = IDX_P2KH
	} else if script.IsP2SH(pkscr) {
		uidx = pkscr[2:22]
		idx = IDX_P2SH
	} else if script.IsP2WPKH(pkscr) {
		uidx = pkscr[2:22]
		idx = IDX_P2WKH
	} else if script.IsP2WSH(pkscr) {
		uidx = pkscr[2:34]
		idx = IDX_P2WSH
	} else if script.IsP2TAP(pkscr) {
		uidx = pkscr[2:34]
		idx = IDX_P2TAP
	}
	return
}

func NewUTXO(tx *utxo.UtxoRec) {
	var uidx []byte
	var idx int
	var rec *OneAllAddrBal
	var nr OneAllAddrInp

	copy(nr[:utxo.UtxoIdxLen], tx.TxID[:]) //RecIdx

	for vout := uint32(0); vout < uint32(len(tx.Outs)); vout++ {
		out := tx.Outs[vout]
		if out == nil {
			continue
		}
		if out.Value < common.AllBalMinVal() {
			continue
		}
		if idx, uidx = Script2Idx(out.PKScr); uidx == nil {
			continue
		}
		rec = AllBalances[idx][string(uidx)]
		if rec == nil {
			rec = &OneAllAddrBal{}
			AllBalances[idx][string(uidx)] = rec
		}

		binary.LittleEndian.PutUint32(nr[utxo.UtxoIdxLen:], vout)

		rec.Value += out.Value

		if rec.unspMap != nil {
			rec.unspMap[nr] = true
			continue
		}
		if len(rec.unsp) >= common.CFG.AllBalances.UseMapCnt-1 {
			// Switch to using map
			rec.unspMap = make(map[OneAllAddrInp]bool, 2*common.CFG.AllBalances.UseMapCnt)
			for _, v := range rec.unsp {
				rec.unspMap[v] = true
			}
			rec.unsp = nil
			rec.unspMap[nr] = true
			continue
		}

		rec.unsp = append(rec.unsp, nr)
	}
}

func all_del_utxos(tx *utxo.UtxoRec, outs []bool) {
	var uidx []byte
	var rec *OneAllAddrBal
	var i, idx int
	var nr OneAllAddrInp
	var ok bool
	copy(nr[:utxo.UtxoIdxLen], tx.TxID[:]) //RecIdx
	for vout, out := range tx.Outs {
		if !outs[vout] || out == nil || out.Value < common.AllBalMinVal() {
			continue
		}
		if idx, uidx = Script2Idx(out.PKScr); uidx == nil {
			continue
		}
		if rec, ok = AllBalances[idx][string(uidx)]; !ok {
			println("ERROR: balance rec not found for", btc.NewAddrFromPkScript(out.PKScr, common.CFG.Testnet).String(),
				btc.NewUint256(tx.TxID[:]).String(), vout, btc.UintToBtc(out.Value))
			continue
		}

		binary.LittleEndian.PutUint32(nr[utxo.UtxoIdxLen:], uint32(vout))

		if rec.unspMap != nil {
			if _, ok := rec.unspMap[nr]; !ok {
				println("ERROR: unspent rec not in map for", btc.NewAddrFromPkScript(out.PKScr, common.CFG.Testnet).String())
				continue
			}
			delete(rec.unspMap, nr)
			if len(rec.unspMap) == 0 {
				delete(AllBalances[idx], string(uidx))
			} else {
				rec.Value -= out.Value
			}
			continue
		}

		if i = slices.Index(rec.unsp, nr); i < 0 {
			println("ERROR: unspent rec not in list for", btc.NewAddrFromPkScript(out.PKScr, common.CFG.Testnet).String())
			continue
		}
		if len(rec.unsp) == 1 {
			delete(AllBalances[idx], string(uidx))
		} else {
			rec.Value -= out.Value
			rec.unsp = append(rec.unsp[:i], rec.unsp[i+1:]...)
		}
	}
}

// TxNotifyAdd is called while accepting the block (from the chain's thread).
func TxNotifyAdd(tx *utxo.UtxoRec) {
	common.CountSafe("BalNotifyAdd")
	AccessMutex.Lock()
	NewUTXO(tx)
	AccessMutex.Unlock()
}

// TxNotifyDel is called while accepting the block (from the chain's thread).
func TxNotifyDel(tx *utxo.UtxoRec, outs []bool) {
	common.CountSafe("BalNotifyDel")
	AccessMutex.Lock()
	all_del_utxos(tx, outs)
	AccessMutex.Unlock()
}

// Call the cb function for each unspent record
func (r *OneAllAddrBal) Browse(cb func(*OneAllAddrInp)) {
	if r.unspMap != nil {
		for v := range r.unspMap {
			cb(&v)
		}
	} else {
		for _, v := range r.unsp {
			cb(&v)
		}
	}
}

func (r *OneAllAddrBal) Count() int {
	if r.unspMap != nil {
		return len(r.unspMap)
	} else {
		return len(r.unsp)
	}
}

func GetAllUnspent(aa *btc.BtcAddr) (thisbal utxo.AllUnspentTx) {
	var rec *OneAllAddrBal

	if aa.SegwitProg != nil {
		if aa.SegwitProg.Version == 1 && len(aa.SegwitProg.Program) == 32 {
			rec = AllBalances[IDX_P2TAP][string(aa.SegwitProg.Program)]
		} else {
			if aa.SegwitProg.Version != 0 {
				return
			}
			switch len(aa.SegwitProg.Program) {
			case 20:
				copy(aa.Hash160[:], aa.SegwitProg.Program)
				rec = AllBalances[IDX_P2WKH][string(aa.Hash160[:])]
			case 32:
				rec = AllBalances[IDX_P2WSH][string(aa.SegwitProg.Program)]
			default:
				return
			}
		}
	} else if aa.Version == btc.AddrVerPubkey(common.Testnet) {
		rec = AllBalances[IDX_P2KH][string(aa.Hash160[:])]
	} else if aa.Version == btc.AddrVerScript(common.Testnet) {
		rec = AllBalances[IDX_P2SH][string(aa.Hash160[:])]
	} else {
		return
	}
	if rec != nil {
		rec.Browse(func(v *OneAllAddrInp) {
			if qr, vout := v.GetRec(); qr != nil {
				if oo := qr.Outs[vout]; oo != nil {
					unsp := &utxo.OneUnspentTx{TxPrevOut: btc.TxPrevOut{Hash: qr.TxID, Vout: vout},
						Value: oo.Value, MinedAt: qr.InBlock, Coinbase: qr.Coinbase, BtcAddr: aa}

					if int(vout+1) < len(qr.Outs) {
						var msg []byte
						if qr.Outs[vout+1] != nil && len(qr.Outs[vout+1].PKScr) > 1 && qr.Outs[vout+1].PKScr[0] == 0x6a {
							msg = qr.Outs[vout+1].PKScr[1:]
						} else if int(vout+1) != len(qr.Outs) && qr.Outs[len(qr.Outs)-1] != nil &&
							len(qr.Outs[len(qr.Outs)-1].PKScr) > 1 && qr.Outs[len(qr.Outs)-1].PKScr[0] == 0x6a {
							msg = qr.Outs[len(qr.Outs)-1].PKScr[1:]
						}
						if msg != nil {
							_, unsp.Message, _, _ = btc.GetOpcode(msg)
						}
					}
					thisbal = append(thisbal, unsp)
				}
			}
		})
	}
	return
}

func PrintStat() {
	var maps, outs, vals [IDX_CNT]uint64

	fmt.Println("AllBalMinVal:", btc.UintToBtc(common.AllBalMinVal()),
		"  UseMapCnt:", common.CFG.AllBalances.UseMapCnt, "  Saved As:", LAST_SAVED_FNAME)

	for idx := range AllBalances {
		for _, r := range AllBalances[idx] {
			vals[idx] += r.Value
			if r.unspMap != nil {
				maps[idx]++
				outs[idx] += uint64(len(r.unspMap))
			} else {
				outs[idx] += uint64(len(r.unsp))
			}
		}
		fmt.Println("AllBalances", IDX2SYMB[idx], ":", len(AllBalances[idx]), "records,",
			outs[idx], "outputs,", btc.UintToBtc(vals[idx]), "BTC,", maps[idx], "maps")
	}
}
