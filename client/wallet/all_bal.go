package wallet

import (
	"fmt"
	"sync"
	"bytes"
	"encoding/binary"
	"github.com/piotrnar/gocoin/lib/btc"
	"github.com/piotrnar/gocoin/lib/chain"
	"github.com/piotrnar/gocoin/client/common"
)

var (
	AllBalancesP2SH map[[20]byte]*OneAllAddrBal = make(map[[20]byte]*OneAllAddrBal)
	AllBalancesP2KH map[[20]byte]*OneAllAddrBal = make(map[[20]byte]*OneAllAddrBal)
	BalanceMutex sync.Mutex
)

type OneAllAddrInp [chain.UtxoIdxLen+4]byte

type OneAllAddrBal struct {
	Value uint64  // Highest bit of it means P2SH
	Unsp []OneAllAddrInp
}

func (ur *OneAllAddrInp) GetRec() (rec *chain.QdbRec, vout uint32) {
	var ind chain.UtxoKeyType
	copy(ind[:], ur[:])
	v := common.BlockChain.Unspent.HashMap[ind]
	if v != nil {
		vout = binary.LittleEndian.Uint32(ur[chain.UtxoIdxLen:])
		rec = chain.NewQdbRec(ind, v)
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
		NewUTXO(chain.NewQdbRecStatic(k, v))
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

func NewUTXO(tx *chain.QdbRec) {
	var uidx [20]byte
	var rec *OneAllAddrBal
	var nr OneAllAddrInp

	copy(nr[:chain.UtxoIdxLen], tx.TxID[:]) //RecIdx

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

		binary.LittleEndian.PutUint32(nr[chain.UtxoIdxLen:], vout)
		rec.Unsp = append(rec.Unsp, nr)
		rec.Value += out.Value
	}
}

func all_del_utxos(tx *chain.QdbRec, outs []bool) {
	var uidx [20]byte
	var rec *OneAllAddrBal
	var i int
	var nr OneAllAddrInp
	var p2kh bool
	copy(nr[:chain.UtxoIdxLen], tx.TxID[:]) //RecIdx
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
			if bytes.Equal(rec.Unsp[i][:chain.UtxoIdxLen], nr[:chain.UtxoIdxLen]) &&
				binary.LittleEndian.Uint32(rec.Unsp[i][chain.UtxoIdxLen:])==vout {
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
			rec.Unsp = append(rec.Unsp[:i], rec.Unsp[i+1:]...)
		}
	}
}

// This is called while accepting the block (from the chain's thread)
func TxNotifyAdd(tx *chain.QdbRec) {
	BalanceMutex.Lock()
	NewUTXO(tx)
	BalanceMutex.Unlock()
}

// This is called while accepting the block (from the chain's thread)
func TxNotifyDel(tx *chain.QdbRec, outs []bool) {
	BalanceMutex.Lock()
	all_del_utxos(tx, outs)
	BalanceMutex.Unlock()
}

func GetAllUnspent(aa *btc.BtcAddr) (thisbal chain.AllUnspentTx) {
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
					unsp := &chain.OneUnspentTx{TxPrevOut:btc.TxPrevOut{Hash:qr.TxID, Vout:vout},
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
