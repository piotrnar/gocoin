package wierddb

import (
	"fmt"
	"errors"
	"bytes"
	"encoding/hex"
	"github.com/piotrnar/gocoin/btc"
)

const poutIdxLen = 8


type oneUnspent struct {
	prv *btc.TxPrevOut
	out *btc.TxOut
	cnt int
}

func unspentIdx(po *btc.TxPrevOut) (o [poutIdxLen]byte) {
	copy(o[:], po.Hash[:poutIdxLen])
	o[0] ^= byte(po.Vout)
	o[1] ^= byte(po.Vout>>8)
	o[2] ^= byte(po.Vout>>16)
	o[3] ^= byte(po.Vout>>24)
	return
}


func (db BtcDB) UnspentPurge() {
	println("UnspentPurge not implemented")
}


func (db BtcDB) UnspentAdd(po *btc.TxPrevOut, rec *btc.TxOut) (e error) {
	//fmt.Println(" +", po.String())
	cur, ok := db.mapUnspent[unspentIdx(po)]
	if ok {
		cur.cnt++
	} else {
		db.mapUnspent[unspentIdx(po)] = &oneUnspent{prv:po, out:rec, cnt:1}
	}
	return
}

func (db BtcDB) UnspentGet(po *btc.TxPrevOut) (res *btc.TxOut, e error) {
	cur, ok := db.mapUnspent[unspentIdx(po)]
	if ok {
		return cur.out, nil
	}
	return nil, errors.New("UnspentGet: unspent not found")
}

func (db BtcDB) UnspentDel(po *btc.TxPrevOut) (e error) {
	//fmt.Println(" -", po.String())
	idx := unspentIdx(po)
	cur, ok := db.mapUnspent[idx]
	if !ok {
		return errors.New("UnspentDel: unspent not found")
	}
	if cur.cnt>2 {
		cur.cnt--
	} else {
		delete(db.mapUnspent, idx)
	}
	return
}

func (db BtcDB) GetUnspentFromPkScr(scr []byte) (res []btc.OneUnspentTx) {
	for _, v := range db.mapUnspent {
		if bytes.Equal(v.out.Pk_script[:], scr[:]) {
			res = append(res, btc.OneUnspentTx{Value:v.out.Value*uint64(v.cnt), Output: *v.prv})
		}
	}
	return
}

func (db BtcDB) ListUnspent() {
	for _, v := range db.mapUnspent {
		fmt.Println(v.prv.String(), hex.EncodeToString(v.out.Pk_script[:]))
	}
}
