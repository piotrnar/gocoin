package wallet

import (
	"bytes"
	"encoding/binary"
	"fmt"

	"github.com/piotrnar/gocoin/client/common"
	"github.com/piotrnar/gocoin/lib/btc"
	"github.com/piotrnar/gocoin/lib/script"
	"github.com/piotrnar/gocoin/lib/utxo"
)

var (
	AllBalancesP2KH, AllBalancesP2SH, AllBalancesP2WKH map[[20]byte]*OneAllAddrBal
	AllBalancesP2WSH, AllBalancesP2TAP                 map[[32]byte]*OneAllAddrBal
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
	common.BlockChain.Unspent.RWMutex.RLock()
	v := common.BlockChain.Unspent.HashMap[ind]
	common.BlockChain.Unspent.RWMutex.RUnlock()
	if v != nil {
		vout = binary.LittleEndian.Uint32(ur[utxo.UtxoIdxLen:])
		rec = utxo.NewUtxoRec(ind, v)
	}
	return
}

func NewUTXO(tx *utxo.UtxoRec) {
	var uidx [20]byte
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
		if script.IsP2KH(out.PKScr) {
			copy(uidx[:], out.PKScr[3:23])
			rec = AllBalancesP2KH[uidx]
			if rec == nil {
				rec = &OneAllAddrBal{}
				AllBalancesP2KH[uidx] = rec
			}
		} else if script.IsP2SH(out.PKScr) {
			copy(uidx[:], out.PKScr[2:22])
			rec = AllBalancesP2SH[uidx]
			if rec == nil {
				rec = &OneAllAddrBal{}
				AllBalancesP2SH[uidx] = rec
			}
		} else if script.IsP2WPKH(out.PKScr) {
			copy(uidx[:], out.PKScr[2:22])
			rec = AllBalancesP2WKH[uidx]
			if rec == nil {
				rec = &OneAllAddrBal{}
				AllBalancesP2WKH[uidx] = rec
			}
		} else if script.IsP2WSH(out.PKScr) {
			var uidx [32]byte
			copy(uidx[:], out.PKScr[2:34])
			rec = AllBalancesP2WSH[uidx]
			if rec == nil {
				rec = &OneAllAddrBal{}
				AllBalancesP2WSH[uidx] = rec
			}
		} else if script.IsP2TAP(out.PKScr) {
			var uidx [32]byte
			copy(uidx[:], out.PKScr[2:34])
			rec = AllBalancesP2TAP[uidx]
			if rec == nil {
				rec = &OneAllAddrBal{}
				AllBalancesP2TAP[uidx] = rec
			}
		} else {
			continue
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
	var uidx [20]byte
	var uidx32 [32]byte
	var rec *OneAllAddrBal
	var i int
	var nr OneAllAddrInp
	var typ int                            // 0 - P2KH, 1 - P2SH, 2 - P2WKH
	copy(nr[:utxo.UtxoIdxLen], tx.TxID[:]) //RecIdx
	for vout := uint32(0); vout < uint32(len(tx.Outs)); vout++ {
		if !outs[vout] {
			continue
		}
		out := tx.Outs[vout]
		if out == nil {
			continue
		}
		if out.Value < common.AllBalMinVal() {
			continue
		}
		if script.IsP2KH(out.PKScr) {
			typ = 0
			copy(uidx[:], out.PKScr[3:23])
			rec = AllBalancesP2KH[uidx]
		} else if script.IsP2SH(out.PKScr) {
			typ = 1
			copy(uidx[:], out.PKScr[2:22])
			rec = AllBalancesP2SH[uidx]
		} else if script.IsP2WPKH(out.PKScr) {
			typ = 2
			copy(uidx[:], out.PKScr[2:22])
			rec = AllBalancesP2WKH[uidx]
		} else if script.IsP2WSH(out.PKScr) {
			typ = 3
			copy(uidx32[:], out.PKScr[2:34])
			rec = AllBalancesP2WSH[uidx32]
		} else if script.IsP2TAP(out.PKScr) {
			typ = 4
			copy(uidx32[:], out.PKScr[2:34])
			rec = AllBalancesP2TAP[uidx32]
		} else {
			continue
		}

		if rec == nil {
			println("balance rec not found for", btc.NewAddrFromPkScript(out.PKScr, common.CFG.Testnet).String(),
				btc.NewUint256(tx.TxID[:]).String(), vout, btc.UintToBtc(out.Value))
			continue
		}

		binary.LittleEndian.PutUint32(nr[utxo.UtxoIdxLen:], vout)

		if rec.unspMap != nil {
			if _, ok := rec.unspMap[nr]; !ok {
				println("unspent rec not in map for", btc.NewAddrFromPkScript(out.PKScr, common.CFG.Testnet).String())
				continue
			}
			delete(rec.unspMap, nr)
			if len(rec.unspMap) == 0 {
				switch typ {
				case 0:
					delete(AllBalancesP2KH, uidx)
				case 1:
					delete(AllBalancesP2SH, uidx)
				case 2:
					delete(AllBalancesP2WKH, uidx)
				case 3:
					delete(AllBalancesP2WSH, uidx32)
				case 4:
					delete(AllBalancesP2TAP, uidx32)
				}
			} else {
				rec.Value -= out.Value
			}
			continue
		}

		for i = 0; i < len(rec.unsp); i++ {
			if bytes.Equal(rec.unsp[i][:], nr[:]) {
				break
			}
		}
		if i == len(rec.unsp) {
			println("unspent rec not in list for", btc.NewAddrFromPkScript(out.PKScr, common.CFG.Testnet).String())
			continue
		}
		if len(rec.unsp) == 1 {
			switch typ {
			case 0:
				delete(AllBalancesP2KH, uidx)
			case 1:
				delete(AllBalancesP2SH, uidx)
			case 2:
				delete(AllBalancesP2WKH, uidx)
			case 3:
				delete(AllBalancesP2WSH, uidx32)
			case 4:
				delete(AllBalancesP2TAP, uidx32)
			}
		} else {
			rec.Value -= out.Value
			rec.unsp = append(rec.unsp[:i], rec.unsp[i+1:]...)
		}
	}
}

// TxNotifyAdd is called while accepting the block (from the chain's thread).
func TxNotifyAdd(tx *utxo.UtxoRec) {
	NewUTXO(tx)
}

// TxNotifyDel is called while accepting the block (from the chain's thread).
func TxNotifyDel(tx *utxo.UtxoRec, outs []bool) {
	all_del_utxos(tx, outs)
}

// Call the cb function for each unspent record
func (r *OneAllAddrBal) Browse(cb func(*OneAllAddrInp)) {
	if r.unspMap != nil {
		for v, _ := range r.unspMap {
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
		var uidx [32]byte
		if aa.SegwitProg.Version == 1 || len(aa.SegwitProg.Program) == 32 {
			copy(uidx[:], aa.SegwitProg.Program)
			rec = AllBalancesP2TAP[uidx]
		} else {
			if aa.SegwitProg.Version != 0 {
				return
			}
			switch len(aa.SegwitProg.Program) {
			case 20:
				copy(aa.Hash160[:], aa.SegwitProg.Program)
				rec = AllBalancesP2WKH[aa.Hash160]
			case 32:
				copy(uidx[:], aa.SegwitProg.Program)
				rec = AllBalancesP2WSH[uidx]
			default:
				return
			}
		}
	} else if aa.Version == btc.AddrVerPubkey(common.Testnet) {
		rec = AllBalancesP2KH[aa.Hash160]
	} else if aa.Version == btc.AddrVerScript(common.Testnet) {
		rec = AllBalancesP2SH[aa.Hash160]
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
	var p2kh_maps, p2kh_outs, p2kh_vals uint64
	for _, r := range AllBalancesP2KH {
		p2kh_vals += r.Value
		if r.unspMap != nil {
			p2kh_maps++
			p2kh_outs += uint64(len(r.unspMap))
		} else {
			p2kh_outs += uint64(len(r.unsp))
		}
	}

	var p2sh_maps, p2sh_outs, p2sh_vals uint64
	for _, r := range AllBalancesP2SH {
		p2sh_vals += r.Value
		if r.unspMap != nil {
			p2sh_maps++
			p2sh_outs += uint64(len(r.unspMap))
		} else {
			p2sh_outs += uint64(len(r.unsp))
		}
	}

	var p2wkh_maps, p2wkh_outs, p2wkh_vals uint64
	for _, r := range AllBalancesP2WKH {
		p2wkh_vals += r.Value
		if r.unspMap != nil {
			p2wkh_maps++
			p2wkh_outs += uint64(len(r.unspMap))
		} else {
			p2wkh_outs += uint64(len(r.unsp))
		}
	}

	var p2wsh_maps, p2wsh_outs, p2wsh_vals uint64
	for _, r := range AllBalancesP2WSH {
		p2wsh_vals += r.Value
		if r.unspMap != nil {
			p2wsh_maps++
			p2wsh_outs += uint64(len(r.unspMap))
		} else {
			p2wsh_outs += uint64(len(r.unsp))
		}
	}

	var p2tap_maps, p2tap_outs, p2tap_vals uint64
	for _, r := range AllBalancesP2TAP {
		p2tap_vals += r.Value
		if r.unspMap != nil {
			p2tap_maps++
			p2tap_outs += uint64(len(r.unspMap))
		} else {
			p2tap_outs += uint64(len(r.unsp))
		}
	}

	fmt.Println("AllBalMinVal:", btc.UintToBtc(common.AllBalMinVal()), "  UseMapCnt:", common.CFG.AllBalances.UseMapCnt)

	fmt.Println("AllBalancesP2KH: ", len(AllBalancesP2KH), "records,",
		p2kh_outs, "outputs,", btc.UintToBtc(p2kh_vals), "BTC,", p2kh_maps, "maps")

	fmt.Println("AllBalancesP2SH: ", len(AllBalancesP2SH), "records,",
		p2sh_outs, "outputs,", btc.UintToBtc(p2sh_vals), "BTC,", p2sh_maps, "maps")

	fmt.Println("AllBalancesP2WKH: ", len(AllBalancesP2WKH), "records,",
		p2wkh_outs, "outputs,", btc.UintToBtc(p2wkh_vals), "BTC,", p2wkh_maps, "maps")

	fmt.Println("AllBalancesP2WSH: ", len(AllBalancesP2WSH), "records,",
		p2wsh_outs, "outputs,", btc.UintToBtc(p2wsh_vals), "BTC,", p2wsh_maps, "maps")

	fmt.Println("AllBalancesP2TAP: ", len(AllBalancesP2TAP), "records,",
		p2tap_outs, "outputs,", btc.UintToBtc(p2tap_vals), "BTC,", p2tap_maps, "maps")
}
