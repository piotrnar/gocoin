package wallet

import (
	"encoding/binary"
	"fmt"
	"slices"
	"sync"

	"github.com/piotrnar/gocoin/client/common"
	"github.com/piotrnar/gocoin/lib/btc"
	"github.com/piotrnar/gocoin/lib/others/siphash"
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
	IDX2SYMB [IDX_CNT]string = [IDX_CNT]string{"P2KH", "P2SH", "P2WKH", "P2WSH", "P2TAP"}
	IDX2SIZE [IDX_CNT]int    = [IDX_CNT]int{20, 20, 20, 32, 32}
)

type OneAllAddrInp [utxo.UtxoIdxLen + 4]byte

type OneAddrIndex uint64

type OneAllAddrBal struct {
	unspMap map[OneAllAddrInp]bool
	unsp    []OneAllAddrInp
	Value   uint64 // Highest bit of it means P2SH
}

var (
	allBalances [IDX_CNT]map[OneAddrIndex]*OneAllAddrBal
	accessMutex sync.Mutex
	useMapCnt   int
)

func (ur *OneAllAddrInp) GetRec() (rec *utxo.UtxoRec, vout uint32) {
	var ind utxo.UtxoKeyType
	copy(ind[:], ur[:])
	common.BlockChain.Unspent.MapMutex[int(ind[0])].RLock()
	v := common.BlockChain.Unspent.HashMap[int(ind[0])][ind]
	common.BlockChain.Unspent.MapMutex[int(ind[0])].RUnlock()
	if v != nil {
		vout = binary.LittleEndian.Uint32(ur[utxo.UtxoIdxLen:])
		rec = utxo.NewUtxoRec(*v)
	}
	return
}

func ourHash(dat []byte) OneAddrIndex {
	return OneAddrIndex(siphash.Hash(0, 0, dat))
}

func Script2Idx(pkscr []byte) (idx int, uidx OneAddrIndex) {
	if script.IsP2KH(pkscr) {
		uidx = ourHash(pkscr[3:23])
		idx = IDX_P2KH
	} else if script.IsP2SH(pkscr) {
		uidx = ourHash(pkscr[2:22])
		idx = IDX_P2SH
	} else if script.IsP2WPKH(pkscr) {
		uidx = ourHash(pkscr[2:22])
		idx = IDX_P2WKH
	} else if script.IsP2WSH(pkscr) {
		uidx = ourHash(pkscr[2:34])
		idx = IDX_P2WSH
	} else if script.IsP2TAP(pkscr) {
		uidx = ourHash(pkscr[2:34])
		idx = IDX_P2TAP
	} else {
		idx = -1
	}
	return
}

func NewUTXO(tx *utxo.UtxoRec) {
	var uidx OneAddrIndex
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
		if idx, uidx = Script2Idx(out.PKScr); idx < 0 {
			continue
		}
		rec = allBalances[idx][uidx]
		if rec == nil {
			rec = &OneAllAddrBal{}
			allBalances[idx][uidx] = rec
		}

		binary.LittleEndian.PutUint32(nr[utxo.UtxoIdxLen:], vout)

		rec.Value += out.Value

		if rec.unspMap != nil {
			rec.unspMap[nr] = true
			continue
		}
		if len(rec.unsp) >= useMapCnt-1 {
			// Switch to using map
			rec.unspMap = make(map[OneAllAddrInp]bool, 2*useMapCnt)
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
	var uidx OneAddrIndex
	var rec *OneAllAddrBal
	var i, idx int
	var nr OneAllAddrInp
	var ok bool
	copy(nr[:utxo.UtxoIdxLen], tx.TxID[:]) //RecIdx
	for vout, out := range tx.Outs {
		if !outs[vout] || out == nil || out.Value < common.AllBalMinVal() {
			continue
		}
		if idx, uidx = Script2Idx(out.PKScr); idx < 0 {
			continue
		}
		if rec, ok = allBalances[idx][uidx]; !ok {
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
				delete(allBalances[idx], uidx)
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
			delete(allBalances[idx], uidx)
		} else {
			rec.Value -= out.Value
			rec.unsp = append(rec.unsp[:i], rec.unsp[i+1:]...)
		}
	}
}

// TxNotifyAdd is called while accepting the block (from the chain's thread).
func TxNotifyAdd(tx *utxo.UtxoRec) {
	accessMutex.Lock()
	NewUTXO(tx)
	accessMutex.Unlock()
}

// TxNotifyDel is called while accepting the block (from the chain's thread).
func TxNotifyDel(tx *utxo.UtxoRec, outs []bool) {
	accessMutex.Lock()
	all_del_utxos(tx, outs)
	accessMutex.Unlock()
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

// all the records should point to the same BTC address
func (r *OneAllAddrBal) BtcAddr() (ad *btc.BtcAddr) {
	var inp OneAllAddrInp
	if r.unspMap != nil {
		for v := range r.unspMap {
			inp = v
			break
		}
	} else if len(r.unsp) > 0 {
		inp = r.unsp[0]
	} else {
		return
	}
	if rec, vout := inp.GetRec(); rec != nil {
		ad = btc.NewAddrFromPkScript(rec.Outs[vout].PKScr, common.Testnet)
	}
	return
}

func GetAllUnspent(aa *btc.BtcAddr) (thisbal utxo.AllUnspentTx) {
	var rec *OneAllAddrBal
	var uidx OneAddrIndex

	accessMutex.Lock() // in case this function was not called from the main thread
	defer accessMutex.Unlock()

	if aa.SegwitProg != nil {
		if aa.SegwitProg.Version == 1 && len(aa.SegwitProg.Program) == 32 {
			uidx = ourHash(aa.SegwitProg.Program)
			rec = allBalances[IDX_P2TAP][uidx]
		} else {
			if aa.SegwitProg.Version != 0 {
				return
			}
			switch len(aa.SegwitProg.Program) {
			case 20:
				copy(aa.Hash160[:], aa.SegwitProg.Program)
				uidx = ourHash(aa.Hash160[:])
				rec = allBalances[IDX_P2WKH][uidx]
			case 32:
				uidx = ourHash(aa.SegwitProg.Program)
				rec = allBalances[IDX_P2WSH][uidx]
			default:
				return
			}
		}
	} else if aa.Version == btc.AddrVerPubkey(common.Testnet) {
		uidx = ourHash(aa.Hash160[:])
		rec = allBalances[IDX_P2KH][uidx]
	} else if aa.Version == btc.AddrVerScript(common.Testnet) {
		uidx = ourHash(aa.Hash160[:])
		rec = allBalances[IDX_P2SH][uidx]
	} else {
		return
	}
	if rec != nil {
		rec.Browse(func(v *OneAllAddrInp) {
			if qr, vout := v.GetRec(); qr != nil {
				if oo := qr.Outs[vout]; oo != nil {
					unsp := &utxo.OneUnspentTx{TxPrevOut: btc.TxPrevOut{Hash: qr.TxID, Vout: vout},
						Value: oo.Value, MinedAt: qr.InBlock, Coinbase: qr.Coinbase, BtcAddr: aa}

					if !qr.Coinbase && int(vout+1) < len(qr.Outs) {
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

func browse(cb func(addr_type int, addr_hash OneAddrIndex, coins *OneAllAddrBal)) {
	for idx := range allBalances {
		for k, r := range allBalances[idx] {
			cb(idx, k, r)
		}
	}
}

func Browse(cb func(addr_type int, addr_hash OneAddrIndex, coins *OneAllAddrBal)) {
	accessMutex.Lock()
	browse(cb)
	accessMutex.Unlock()
}

func PrintStat() {
	var maps, outs, vals [IDX_CNT]uint64

	accessMutex.Lock()
	fmt.Println("AllBalMinVal:", btc.UintToBtc(common.AllBalMinVal()),
		"  UseMapCnt:", useMapCnt, "  Saved As:", LAST_SAVED_FNAME)

	browse(func(idx int, k OneAddrIndex, r *OneAllAddrBal) {
		vals[idx] += r.Value
		if r.unspMap != nil {
			maps[idx]++
			outs[idx] += uint64(len(r.unspMap))
		} else {
			outs[idx] += uint64(len(r.unsp))
		}
	})
	for idx := range outs {
		fmt.Println("AllBalances", IDX2SYMB[idx], ":", len(allBalances[idx]), "records,",
			outs[idx], "outputs,", btc.UintToBtc(vals[idx]), "BTC,", maps[idx], "maps")
	}
	accessMutex.Unlock()
}
