package wallet

import (
	"encoding/binary"
	"fmt"
	"slices"
	"sync"
	"unsafe"

	"github.com/piotrnar/gocoin/client/common"
	"github.com/piotrnar/gocoin/lib/btc"
	"github.com/piotrnar/gocoin/lib/others/siphash"
	"github.com/piotrnar/gocoin/lib/script"
	"github.com/piotrnar/gocoin/lib/utxo"
	"github.com/syndtr/goleveldb/leveldb"
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
	baldb       *leveldb.DB
	AccessMutex sync.Mutex
	IDX2SYMB    [IDX_CNT]string = [IDX_CNT]string{"P2KH", "P2SH", "P2WKH", "P2WSH", "P2TAP"}
	IDX2SIZE    [IDX_CNT]int    = [IDX_CNT]int{20, 20, 20, 32, 32}
	isFetching  map[string]*OneAllAddrBal
)

type OneAllAddrInp [utxo.UtxoIdxLen + 4]byte

type OneAllAddrBal struct {
	Value uint64 // Highest bit of it means P2SH
	unsp  []OneAllAddrInp
}

func (ur *OneAllAddrInp) GetRec() (rec *utxo.UtxoRec, vout uint32) {
	if v := common.BlockChain.Unspent.GetByKey(ur[:utxo.UtxoIdxLen]); v != nil {
		vout = binary.LittleEndian.Uint32(ur[utxo.UtxoIdxLen:])
		rec = utxo.NewUtxoRec(*(*utxo.UtxoKeyType)(unsafe.Pointer(&ur[0])), v)
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

func (b *OneAllAddrBal) Serialize() (res []byte) {
	res = make([]byte, 8+(utxo.UtxoIdxLen+4)*len(b.unsp))
	binary.LittleEndian.PutUint64(res[:], b.Value)
	offs := 8
	for _, u := range b.unsp {
		copy(res[offs:], u[:])
		offs += utxo.UtxoIdxLen + 4
	}
	return
}

func newAddrBal(v []byte) (res *OneAllAddrBal) {
	res = new(OneAllAddrBal)
	res.Value = binary.LittleEndian.Uint64(v)
	res.unsp = make([]OneAllAddrInp, 0, (len(v)-8)/(utxo.UtxoIdxLen+4))
	for offs := 8; offs < len(v); offs += utxo.UtxoIdxLen + 4 {
		var u OneAllAddrInp
		copy(u[:], v[offs:])
		res.unsp = append(res.unsp, u)

	}
	return
}

func idx2key(idx int, uidx []byte) (kk []byte) {
	kk = make([]byte, 8)
	binary.LittleEndian.PutUint64(kk, siphash.Hash(0, 0, uidx))
	kk[0] = byte(idx)

	/*kk = make([]byte, len(uidx)+1)
	kk[0] = byte(idx)
	copy(kk[1:], uidx)*/

	//kk = append(uidx, byte(idx))
	return
}

func db_get(idx int, uidx []byte) (res *OneAllAddrBal) {
	if isFetching != nil {
		return isFetching[string(idx2key(idx, uidx))]
	}
	if v, er := baldb.Get(idx2key(idx, uidx), nil); er == nil {
		res = newAddrBal(v)
	}
	return
}

func db_put(idx int, uidx []byte, rec *OneAllAddrBal) {
	if isFetching != nil {
		isFetching[string(idx2key(idx, uidx))] = rec
	} else {
		baldb.Put(idx2key(idx, uidx), rec.Serialize(), nil)
	}
}

func db_del(idx int, uidx []byte) {
	if isFetching != nil {
		panic("should not be called when fetching")
	}
	baldb.Delete(idx2key(idx, uidx), nil)
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
		rec = db_get(idx, uidx)
		if rec == nil {
			rec = &OneAllAddrBal{}
		}

		binary.LittleEndian.PutUint32(nr[utxo.UtxoIdxLen:], vout)

		rec.Value += out.Value

		rec.unsp = append(rec.unsp, nr)
		db_put(idx, uidx, rec)
	}
}

func all_del_utxos(tx *utxo.UtxoRec, outs []bool) {
	var uidx []byte
	var rec *OneAllAddrBal
	var i, idx int
	var nr OneAllAddrInp
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
		if idx, uidx = Script2Idx(out.PKScr); uidx == nil {
			continue
		}
		rec = db_get(idx, uidx)

		if rec == nil {
			println("balance rec not found for", btc.NewAddrFromPkScript(out.PKScr, common.CFG.Testnet).String(),
				btc.NewUint256(tx.TxID[:]).String(), vout, btc.UintToBtc(out.Value))
			continue
		}

		binary.LittleEndian.PutUint32(nr[utxo.UtxoIdxLen:], vout)

		if i = slices.Index(rec.unsp, nr); i < 0 {
			println("unspent rec not in list for", btc.NewAddrFromPkScript(out.PKScr, common.CFG.Testnet).String())
			continue
		}
		if len(rec.unsp) == 1 {
			db_del(idx, uidx)
		} else {
			rec.Value -= out.Value
			rec.unsp = slices.Delete(rec.unsp, i, i+1)
			db_put(idx, uidx, rec)
		}
	}
}

// TxNotifyAdd is called while accepting the block (from the chain's thread).
func TxNotifyAdd(tx *utxo.UtxoRec) {
	AccessMutex.Lock()
	NewUTXO(tx)
	AccessMutex.Unlock()
}

// TxNotifyDel is called while accepting the block (from the chain's thread).
func TxNotifyDel(tx *utxo.UtxoRec, outs []bool) {
	AccessMutex.Lock()
	all_del_utxos(tx, outs)
	AccessMutex.Unlock()
}

// Call the cb function for each unspent record
func (r *OneAllAddrBal) Browse(cb func(*OneAllAddrInp)) {
	for _, v := range r.unsp {
		cb(&v)
	}
}

func (r *OneAllAddrBal) Count() int {
	return len(r.unsp)
}

func GetAllUnspent(aa *btc.BtcAddr) (thisbal utxo.AllUnspentTx) {
	var rec *OneAllAddrBal

	if aa.SegwitProg != nil {
		if aa.SegwitProg.Version == 1 && len(aa.SegwitProg.Program) == 32 {
			rec = db_get(IDX_P2TAP, aa.SegwitProg.Program)
		} else {
			if aa.SegwitProg.Version != 0 {
				return
			}
			switch len(aa.SegwitProg.Program) {
			case 20:
				copy(aa.Hash160[:], aa.SegwitProg.Program)
				rec = db_get(IDX_P2WKH, aa.Hash160[:])
			case 32:
				rec = db_get(IDX_P2WSH, aa.SegwitProg.Program)
			default:
				return
			}
		}
	} else if aa.Version == btc.AddrVerPubkey(common.Testnet) {
		rec = db_get(IDX_P2KH, aa.Hash160[:])
	} else if aa.Version == btc.AddrVerScript(common.Testnet) {
		rec = db_get(IDX_P2SH, aa.Hash160[:])
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
	var outs, vals, cnt [IDX_CNT]uint64

	fmt.Println("AllBalMinVal:", btc.UintToBtc(common.AllBalMinVal()))
	iter := baldb.NewIterator(nil, nil)
	defer iter.Release()
	for iter.Next() {
		k := iter.Key()
		if len(k) < 8 {
			println("skipping", string(k))
			continue
		}
		v := iter.Value()
		idx := k[0]
		cnt[idx]++
		vals[idx] += binary.LittleEndian.Uint64(v)
		outs[idx] += uint64(len(v)-8) / (utxo.UtxoIdxLen + 4)
	}
	for idx := range outs {
		fmt.Println("AllBalances", IDX2SYMB[idx], ":", cnt[idx], "records,",
			outs[idx], "outputs,", btc.UintToBtc(vals[idx]), "BTC,")
	}
}
