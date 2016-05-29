package wallet

import (
	"bytes"
	"encoding/binary"
	"github.com/piotrnar/gocoin/lib/btc"
	"github.com/piotrnar/gocoin/lib/chain"
	"github.com/piotrnar/gocoin/client/common"
)


const MIN_VAL = 1
const VALUE_P2SH_BIT = 1<<63

var (
	AllBalances map[[20]byte]*OneAllAddrBal = make(map[[20]byte]*OneAllAddrBal)
)

type OneAllAddrInp [1+8+4]byte

type OneAllAddrBal struct {
	Value uint64  // Highest bit of it means P2SH
	Unsp []OneAllAddrInp
}

func all_add_utxo(tx *chain.QdbRec) {
	var uidx [20]byte
	var rec *OneAllAddrBal
	var nr OneAllAddrInp
	var p2sh bool

	nr[0] = byte(tx.TxID[31]) % chain.NumberOfUnspentSubDBs //DbIdx
	copy(nr[1:9], tx.TxID[:8]) //RecIdx

	for vout:=uint32(0); vout<uint32(len(tx.Outs)); vout++ {
		out := tx.Outs[vout]
		if out == nil {
			continue
		}
		if out.Value < MIN_VAL {
			continue
		}
		if out.IsP2KH() {
			copy(uidx[:], out.PKScr[3:23])
			p2sh = false
		} else if out.IsP2SH() {
			copy(uidx[:], out.PKScr[2:22])
			p2sh = true
		} else {
			continue
		}
		if rec=AllBalances[uidx]; rec==nil {
			rec = &OneAllAddrBal{}
			if p2sh {
				rec.Value = VALUE_P2SH_BIT
			}
			AllBalances[uidx] = rec
		}
		binary.LittleEndian.PutUint32(nr[9:13], vout)
		rec.Unsp = append(rec.Unsp, nr)
		rec.Value += out.Value
	}
}

func all_del_utxos(tx *chain.QdbRec, outs []bool) {
	var uidx [20]byte
	var rec *OneAllAddrBal
	var i int
	var nr OneAllAddrInp
	nr[0] = byte(tx.TxID[31]) % chain.NumberOfUnspentSubDBs //DbIdx
	copy(nr[1:9], tx.TxID[:8]) //RecIdx
	for vout:=uint32(0); vout<uint32(len(tx.Outs)); vout++ {
		if !outs[vout] {
			continue
		}
		out := tx.Outs[vout]
		if out == nil {
			continue
		}
		if out.Value < MIN_VAL {
			continue
		}
		if out.IsP2KH() {
			copy(uidx[:], out.PKScr[3:23])
		} else if out.IsP2SH() {
			copy(uidx[:], out.PKScr[2:22])
		} else {
			continue
		}

		rec = AllBalances[uidx]
		if rec==nil {
			println("balance rec not found for", btc.NewAddrFromPkScript(out.PKScr, common.CFG.Testnet).String())
			continue
		}

		for i=0; i<len(rec.Unsp); i++ {
			if bytes.Equal(rec.Unsp[i][:9], nr[:9]) && binary.LittleEndian.Uint32(rec.Unsp[i][9:13])==vout {
				break
			}
		}
		if i==len(rec.Unsp) {
			println("unspent rec not found for", btc.NewAddrFromPkScript(out.PKScr, common.CFG.Testnet).String())
			continue
		}
		if len(rec.Unsp)==1 {
			delete(AllBalances, uidx)
		} else {
			rec.Value -= out.Value
			rec.Unsp = append(rec.Unsp[:i], rec.Unsp[i+1:]...)
		}
	}
}
