package btc

import (
	"fmt"
)

type UnspentDb struct {
	db BtcDB

	unwd *txUnwindData
	lastPickInput *TxPrevOut
	lastPickOut *TxOut
}

func NewUnspentDb(db BtcDB) (us *UnspentDb) {
	us = new(UnspentDb)
	us.db = db
	us.unwd = NewUnwindBuffer(us)
	return
}

func (us *UnspentDb)NewHeight(height uint32) {
	if don(DBG_UNSPENT) {
		fmt.Printf("NewHeight %d\n", height)
	}
	us.unwd.NewHeight(height)
}


func (us *UnspentDb)del(txin *TxPrevOut) {
	e := us.db.UnspentDel(txin)
	if e != nil {
		panic(e.Error())
	}
}


func (us *UnspentDb)add(txin *TxPrevOut, newout *TxOut) {
	e := us.db.UnspentAdd(txin, newout)
	if e != nil {
		panic(e.Error())
	}
}


func (us *UnspentDb)LookUnspent(txin *TxPrevOut) (txout *TxOut) {
	txo, e := us.db.UnspentGet(txin)
	if e != nil {
		panic(e.Error())
	}
	txout = txo
	return
}

func (us *UnspentDb)PickUnspent(txin *TxPrevOut) (*TxOut, bool) {
	txo, e := us.db.UnspentGet(txin)
	if e!=nil || txo==nil {
		return nil, false
	}
	us.lastPickInput = txin
	us.lastPickOut = txo
	return txo, true
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


type OneUnspentTx struct {
	Output TxPrevOut
	Value uint64
}

func (u *OneUnspentTx) String() string {
	return u.Output.String() + fmt.Sprintf(" %15.8f BTC", float64(u.Value)/1e8)
}


func (us *UnspentDb) GetUnspentFromPkScr(scr []byte) (res []OneUnspentTx) {
	/*
	for _, v := range us.outs {
		if bytes.Equal(v.txout.Pk_script[:], scr[:]) {
			res = append(res, OneUnspentTx{Value:v.txout.Value, Output: v.prvout})
		}
	}
	*/
	return
}
