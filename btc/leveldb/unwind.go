package leveldb

import (
	"errors"
	"github.com/piotrnar/gocoin/btc"
)

type oneUnwind struct {
	prv *btc.TxPrevOut
	out *btc.TxOut
}

type oneUnwindSet struct {
	added []oneUnwind
	deled []oneUnwind
}

func (db BtcDB) UnwindAdd(height uint32, added int, po *btc.TxPrevOut, rec *btc.TxOut) (e error) {
	cur, ok := db.mapUnwind[height]
	if !ok {
		cur = new(oneUnwindSet)
		db.mapUnwind[height] = cur
	}
	
	if added!=0 {
		cur.added = append(cur.added, oneUnwind{prv:po, out:rec})
	} else {
		cur.deled = append(cur.deled, oneUnwind{prv:po, out:rec})
	}
	return
}

func (db BtcDB) UnwindDel(height uint32) (e error) {
	delete(db.mapUnwind, height)
	return
}

func (db BtcDB) UnwindNow(height uint32) (e error) {
	dat, ok := db.mapUnwind[height]
	if !ok {
		return errors.New("UnwindNow: no such data")
	}

	for i := range dat.deled {
		db.UnspentAdd(dat.deled[i].prv, dat.deled[i].out)
	}
	
	for i := range dat.added {
		db.UnspentDel(dat.added[i].prv)
	}

	delete(db.mapUnwind, height)
	return
}

