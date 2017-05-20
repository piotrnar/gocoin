package wallet

import (
	"fmt"
	"bytes"
	"encoding/binary"
	"github.com/piotrnar/gocoin/lib/btc"
	"github.com/piotrnar/gocoin/lib/utxo"
	"github.com/piotrnar/gocoin/lib/chain"
	"github.com/piotrnar/gocoin/client/common"
)

var (
	AllBalancesP2SH map[[20]byte]*OneAllAddrBal = make(map[[20]byte]*OneAllAddrBal)
	AllBalancesP2KH map[[20]byte]*OneAllAddrBal = make(map[[20]byte]*OneAllAddrBal)

	dirty_list_p2sh [][20]byte
	dirty_list_p2kh [][20]byte
)

type OneAllAddrInp [utxo.UtxoIdxLen+4]byte

type OneAllAddrBal struct {
	Value uint64  // Highest bit of it means P2SH
	Dirty int
	Unsp []OneAllAddrInp
}

func (ur *OneAllAddrInp) GetRec() (rec *utxo.UtxoRec, vout uint32) {
	var ind utxo.UtxoKeyType
	copy(ind[:], ur[:])
	v := common.BlockChain.Unspent.HashMap[ind]
	if v != nil {
		vout = binary.LittleEndian.Uint32(ur[utxo.UtxoIdxLen:])
		rec = utxo.NewUtxoRec(ind, utxo.Slice(v))
	}
	return
}

func FetchInitialBalance() {
	var cur_rec, cnt_dwn, perc int
	cnt_dwn_from := len(common.BlockChain.Unspent.HashMap)/100
	info := "Loading balance of P2SH/P2KH outputs of " + btc.UintToBtc(common.AllBalMinVal) + " BTC or more"
	for k, v := range common.BlockChain.Unspent.HashMap {
		if chain.AbortNow {
			break
		}
		NewUTXO(utxo.NewUtxoRecStatic(k, utxo.Slice(v)))
		cur_rec++
		if cnt_dwn==0 {
			fmt.Print("\r", info, " - ", perc, "% complete ... ")
			cnt_dwn = cnt_dwn_from
			perc++
		} else {
			cnt_dwn--
		}
	}
	fmt.Print("\r                                                                                  \r")
}

func NewUTXO(tx *utxo.UtxoRec) {
	var uidx [20]byte
	var rec *OneAllAddrBal
	var nr OneAllAddrInp

	copy(nr[:utxo.UtxoIdxLen], tx.TxID[:]) //RecIdx

	for vout:=uint32(0); vout<uint32(len(tx.Outs)); vout++ {
		out := tx.Outs[vout]
		if out == nil {
			continue
		}
		if out.Value < common.AllBalMinVal {
			continue
		}
		if out.IsP2KH() {
			copy(uidx[:], out.PKScr[3:23])
			rec = AllBalancesP2KH[uidx]
			if rec==nil {
				rec = &OneAllAddrBal{}
				AllBalancesP2KH[uidx] = rec
			}
		} else if out.IsP2SH() {
			copy(uidx[:], out.PKScr[2:22])
			rec = AllBalancesP2SH[uidx]
			if rec==nil {
				rec = &OneAllAddrBal{}
				AllBalancesP2SH[uidx] = rec
			}
		} else {
			continue
		}

		binary.LittleEndian.PutUint32(nr[utxo.UtxoIdxLen:], vout)
		rec.Unsp = append(rec.Unsp, nr)
		rec.Value += out.Value
	}
}

func all_del_utxos(tx *utxo.UtxoRec, outs []bool) {
	var uidx [20]byte
	var rec *OneAllAddrBal
	var i int
	var nr OneAllAddrInp
	var p2kh bool
	copy(nr[:utxo.UtxoIdxLen], tx.TxID[:]) //RecIdx
	for vout:=uint32(0); vout<uint32(len(tx.Outs)); vout++ {
		if !outs[vout] {
			continue
		}
		out := tx.Outs[vout]
		if out == nil {
			continue
		}
		if out.Value < common.AllBalMinVal {
			continue
		}
		if p2kh=out.IsP2KH(); p2kh {
			copy(uidx[:], out.PKScr[3:23])
			rec = AllBalancesP2KH[uidx]
		} else if out.IsP2SH() {
			copy(uidx[:], out.PKScr[2:22])
			rec = AllBalancesP2SH[uidx]
		} else {
			continue
		}

		if rec==nil {
			println("balance rec not found for", btc.NewAddrFromPkScript(out.PKScr, common.CFG.Testnet).String())
			continue
		}

		for i=0; i<len(rec.Unsp); i++ {
			if bytes.Equal(rec.Unsp[i][:utxo.UtxoIdxLen], nr[:utxo.UtxoIdxLen]) &&
				binary.LittleEndian.Uint32(rec.Unsp[i][utxo.UtxoIdxLen:])==vout {
				break
			}
		}
		if i==len(rec.Unsp) {
			println("unspent rec not found for", btc.NewAddrFromPkScript(out.PKScr, common.CFG.Testnet).String())
			continue
		}
		if len(rec.Unsp)==1 {
			if p2kh {
				delete(AllBalancesP2KH, uidx)
			} else {
				delete(AllBalancesP2SH, uidx)
			}
		} else {
			rec.Value -= out.Value

			binary.LittleEndian.PutUint32(rec.Unsp[i][utxo.UtxoIdxLen:], 0xffffffff)
			rec.Dirty++

			if p2kh {
				dirty_list_p2kh = append(dirty_list_p2kh, uidx)
			} else {
				dirty_list_p2sh = append(dirty_list_p2sh, uidx)
			}
		}
	}
}


func block_finished(l [][20]byte, m map[[20]byte]*OneAllAddrBal) {
		var i int
	for _, k := range l {
		rec := m[k]
		if rec==nil || rec.Dirty==0 {
			continue // already processed
		}
		if rec.Dirty==len(rec.Unsp) {
			delete(m, k)
		} else {
			newunsp := make([]OneAllAddrInp, len(rec.Unsp)-rec.Dirty)
			i = 0
			for _, v := range rec.Unsp {
				if binary.LittleEndian.Uint32(v[utxo.UtxoIdxLen:])!=0xffffffff {
					newunsp[i] = v
					i++
				}
			}
			rec.Unsp = newunsp
			rec.Dirty = 0
		}
	}
}

// This is called after a new block was accepted, to defrag all the "dirty" outputs
func BlockFinished(h uint32) {
	block_finished(dirty_list_p2sh, AllBalancesP2SH)
	dirty_list_p2sh = nil
	block_finished(dirty_list_p2kh, AllBalancesP2KH)
	dirty_list_p2kh = nil
}

// This is called while accepting the block (from the chain's thread)
func TxNotifyAdd(tx *utxo.UtxoRec) {
	NewUTXO(tx)
}

// This is called while accepting the block (from the chain's thread)
func TxNotifyDel(tx *utxo.UtxoRec, outs []bool) {
	all_del_utxos(tx, outs)
}

func GetAllUnspent(aa *btc.BtcAddr) (thisbal utxo.AllUnspentTx) {
	var rec *OneAllAddrBal
	if aa.Version==btc.AddrVerPubkey(common.Testnet) {
		rec = AllBalancesP2KH[aa.Hash160]
	} else if aa.Version==btc.AddrVerScript(common.Testnet) {
		rec = AllBalancesP2SH[aa.Hash160]
	} else {
		return
	}
	if rec!=nil {
		for _, v := range rec.Unsp {
			if qr, vout := v.GetRec(); qr!=nil {
				if oo := qr.Outs[vout]; oo!=nil {
					unsp := &utxo.OneUnspentTx{TxPrevOut:btc.TxPrevOut{Hash:qr.TxID, Vout:vout},
						Value:oo.Value, MinedAt:qr.InBlock, Coinbase:qr.Coinbase, BtcAddr:aa}

					if int(vout+1) < len(qr.Outs) {
						var msg []byte
						if qr.Outs[vout+1]!=nil && len(qr.Outs[vout+1].PKScr)>1 && qr.Outs[vout+1].PKScr[0]==0x6a {
							msg = qr.Outs[vout+1].PKScr[1:]
						} else if int(vout+1)!=len(qr.Outs) && qr.Outs[len(qr.Outs)-1]!=nil &&
							len(qr.Outs[len(qr.Outs)-1].PKScr)>1 && qr.Outs[len(qr.Outs)-1].PKScr[0]==0x6a {
							msg = qr.Outs[len(qr.Outs)-1].PKScr[1:]
						}
						if msg!=nil {
							_, unsp.Message, _, _ = btc.GetOpcode(msg)
						}
					}
					thisbal = append(thisbal, unsp)
				}
			}
		}
	}
	return
}
