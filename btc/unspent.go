package btc

import (
	"fmt"
	"os"
	"bytes"
)

type unspentTxOut struct {
	txout TxOut
	prvout TxPrevOut
	times byte
}

const txOutMapLen = 8  // The bigger it it, the more memory is needed, but lower chance of a collision


type UnspentDb struct {
	outs map[[txOutMapLen]byte] unspentTxOut
	unwd *txUnwindData
	lastPickInput *TxPrevOut
	lastPickOut *TxOut
}

func NewUnspentDb() (us *UnspentDb) {
	us = new(UnspentDb)
	us.outs = make(map[[txOutMapLen]byte] unspentTxOut, UnspentTxsMapInitLen)
	us.unwd = NewUnwindBuffer()
	return
}

func NewTxoutUnspentIndex(po *TxPrevOut) (o [txOutMapLen]byte) {
	put32lsb(o[:], po.Vout)
	for i := 0; i<txOutMapLen; i++ {
		o[i] ^= po.Hash[i]
	}
	return 
}


func (us *UnspentDb)NewHeight(height uint32) {
	if don(DBG_UNSPENT) {
		fmt.Printf("NewHeight %d\n", height)
	}
	us.unwd.NewHeight(height)
}


func (us *UnspentDb)del(txin *TxPrevOut) {
	idx := NewTxoutUnspentIndex(txin)
	is, pres := us.outs[idx]
	if !pres {
		println("Cannot remove", txin.String())
		os.Exit(1)
	}
	if is.times > 1 {
		is.times--
	} else {
		delete(us.outs, idx)
	}
}


func (us *UnspentDb)add(txin *TxPrevOut, newout *TxOut) {
	idx := NewTxoutUnspentIndex(txin)
	is, pres := us.outs[idx]
	if pres {
		fmt.Println("Note:", txin.String(), "is already unspent")
		is.times++
	} else {
		rec := unspentTxOut{txout:*newout,prvout:*txin,times:1}
		us.outs[idx] = rec
	}
}


func (us *UnspentDb)LookUnspent(txin *TxPrevOut) (txout *TxOut) {
	idx := NewTxoutUnspentIndex(txin)
	txout_x, ok := us.outs[idx]
	if ok {
		txout = &txout_x.txout
	}
	return
}

func (us *UnspentDb)PickUnspent(txin *TxPrevOut) (txout *TxOut, ok bool) {
	idx := NewTxoutUnspentIndex(txin)
	txout_x, yes := us.outs[idx]
	if yes {
		txout = &txout_x.txout
		us.lastPickInput = txin
		us.lastPickOut = txout
		ok = true
	}
	return
}


func (us *UnspentDb)RemoveLastPick(height uint32) {
	if don(DBG_UNSPENT) {
		fmt.Println(" - ", us.lastPickInput.String())
	}
	us.unwd.addToDeleted(height, us.lastPickInput, us.lastPickOut)
	us.del(us.lastPickInput)
	us.lastPickInput = nil
	us.lastPickOut = nil
}


// We down want txin to be pointer since it gets changed (a local variable) in the caller
func (us *UnspentDb)Append(height uint32, txin TxPrevOut, newout *TxOut) {
	if don(DBG_UNSPENT) {
		fmt.Println(" + ", txin.String())
	}
	us.unwd.addToAdded(height, &txin, newout)
	us.add(&txin, newout)
}

func (us *UnspentDb)UnwindBlock(height uint32) {
	us.unwd.UnwindBlock(height, us)
}


func (us *UnspentDb)Stats() (s string) {
	siz := uint64(0)
	sum := uint64(0)            
	cnt := uint32(0)
	for _, v := range us.outs {
		cnt += uint32(v.times)
		sum += v.txout.Value*uint64(v.times)
		siz += uint64(v.txout.Size() + 32 + 4) * uint64(v.times)
	}
	s += fmt.Sprintf("UNSPENT : tx_cnt=%d  btc=%.8f  size:%dMB\n", cnt, float64(sum)/1e8, siz>>20)
	s += us.unwd.Stats()
	return
}


func (us *UnspentDb)Load(f *os.File) {
	us.outs = make(map[[txOutMapLen]byte] unspentTxOut, UnspentTxsMapInitLen)
	var txout TxOut
	var prvout TxPrevOut
	for prvout.Load(f) {
		txout.Load(f)
		us.add(&prvout, &txout)
	}
	println(len(us.outs), "loaded into UnspentDb")
}


func (us *UnspentDb) Save(f *os.File) {
	for _, v := range us.outs {
		for xx := 0; xx<int(v.times); xx++ {
			v.prvout.Save(f)
			v.txout.Save(f)
		}
	}
	println(len(us.outs), "saved in UnspentDb", getfilepos(f))
}


type OneUnspentTx struct {
	Output TxPrevOut
	Value uint64
}

func (u *OneUnspentTx) String() string {
	return u.Output.String() + fmt.Sprintf(" %15.8f BTC", float64(u.Value)/1e8)
}


func (us *UnspentDb) GetUnspentFromPkScr(scr []byte) (res []OneUnspentTx) {
	for _, v := range us.outs {
		if bytes.Equal(v.txout.Pk_script[:], scr[:]) {
			res = append(res, OneUnspentTx{Value:v.txout.Value, Output: v.prvout})
		}
	}
	return
}
