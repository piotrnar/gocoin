package btc

import (
	"fmt"
	"os"
)

type UnspentDb struct {
	outs map[TxPrevOut] *TxOut
	unwd *txUnwindData
	lastPickInput *TxPrevOut
	lastPickOut *TxOut
}

func NewUnspentDb() (us *UnspentDb) {
	us = new(UnspentDb)
	us.outs = make(map[TxPrevOut] *TxOut, UnspentTxsMapInitLen)
	us.unwd = NewUnwindBuffer()
	return
}

func (us *UnspentDb)NewHeight(height uint32) {
	if don(DBG_UNSPENT) {
		fmt.Printf("NewHeight %d\n", height)
	}
	us.unwd.NewHeight(height)
}


func (us *UnspentDb)del(txin *TxPrevOut) {
	is, pres := us.outs[*txin]
	if !pres {
		println("Cannot remove", txin.String())
		os.Exit(1)
	}
	if is.Times > 1 {
		is.Times--
	} else {
		delete(us.outs, *txin)
	}
}


func (us *UnspentDb)add(txin *TxPrevOut, newout *TxOut) {
	is, pres := us.outs[*txin]
	if pres {
		fmt.Println("Note:", txin.String(), "is already unspent")
		is.Times++
	} else {
		newout.Times = 1
		us.outs[*txin] = newout
	}
}


func (us *UnspentDb)LookUnspent(txin *TxPrevOut) (txout *TxOut) {
	txout, _ = us.outs[*txin]
	return
}

func (us *UnspentDb)PickUnspent(txin *TxPrevOut) (txout *TxOut, ok bool) {
	txout, ok = us.outs[*txin]
	if ok {
		us.lastPickInput = txin
		us.lastPickOut = txout
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
	sum := uint64(0)
	cnt := uint32(0)
	for _, v := range us.outs {
		cnt += v.Times
		if v.Times > 1 {
			sum += v.Value*uint64(v.Times)
		} else {
			sum += v.Value
		}
	}
	s += fmt.Sprintf("UNSPENT : tx_cnt=%d  btc=%.8f\n", cnt, float64(sum)/1e8)
	s += us.unwd.Stats()
	return
}

func (us *UnspentDb)Save() {
	// Variable record length
	f, e := os.Create("unspent_outs.bin")
	if e == nil {
		for k, v := range us.outs {
			f.Write(k.Hash[:])
			write32bit(f, k.Index)
			v.Save(f)
		}
		println(len(us.outs), "saved in unspent_outs.bin")
		f.Close()
	}
	
	us.unwd.Save()
}

