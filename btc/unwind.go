package btc

import (
	"fmt"
	"os"
)

type UnspentOneUnwind struct {
	input *TxPrevOut
	out *TxOut
}

type UnspentBlockUnwind struct {
	added []UnspentOneUnwind
	deleted []UnspentOneUnwind
}


type txUnwindData struct {
	blocksUnwind map[uint32]*UnspentBlockUnwind
}


func (o *UnspentOneUnwind)Load(f *os.File) {
	o.input = new(TxPrevOut)
	o.out = new(TxOut)
	o.input.Load(f)
	o.out.Load(f)
}

func (o *UnspentOneUnwind)Save(f *os.File) {
	o.input.Save(f)
	o.out.Save(f)
}


func NewUnwindBuffer() (ub *txUnwindData) {
	ub = new(txUnwindData)
	ub.blocksUnwind = make(map[uint32] *UnspentBlockUnwind, UnspentTxsMapInitLen)
	return 
}

func (u *txUnwindData)NewHeight(height uint32) {
	if height > UnwindBufferMaxHistory {
		delete(u.blocksUnwind, height-UnwindBufferMaxHistory)
	}
	
	_, pres := u.blocksUnwind[height]
	if !pres {
		u.blocksUnwind[height] = new(UnspentBlockUnwind)
	} else {
		fmt.Printf("*** Height %d already present in the blocksUnwind buffer\n", height)
		os.Exit(1)
	}
}

func (u *txUnwindData)addToDeleted(height uint32, txin *TxPrevOut, txout *TxOut) {
	unw, pres := u.blocksUnwind[height]
	if !pres {
		fmt.Printf("addToDeleted: Height %d not present in the blocksUnwind buffer\n", height)
		os.Exit(1)
	}
	unw.deleted = append(unw.deleted, UnspentOneUnwind{input:txin, out:txout})
}


func (u *txUnwindData)addToAdded(height uint32, txin *TxPrevOut, newout *TxOut) {
	unw, pres := u.blocksUnwind[height]
	if !pres {
		fmt.Printf("addToAdded: Height %d not present in the blocksUnwind buffer\n", height)
		os.Exit(1)
	}
	unw.added = append(unw.added, UnspentOneUnwind{input:txin, out:newout})
}


func (u *txUnwindData)UnwindBlock(height uint32, db *UnspentDb) {
	unw, pres := u.blocksUnwind[height]
	if !pres {
		fmt.Println("unwind data not present for block", height)
		os.Exit(1)
	}

	fmt.Printf("Unwiding tx from block %d: %d adds, %d dels\n", height, len(unw.added), len(unw.deleted))
	if don(DBG_UNSPENT) {
		for i := range unw.deleted {
			fmt.Println(" del:", unw.deleted[i].input.String())
		}
		for i := range unw.added {
			fmt.Println(" add:", unw.added[i].input.String())
		}
	}
	for i := range unw.deleted {
		db.add(unw.deleted[i].input, unw.deleted[i].out)
	}
	
	for i := range unw.added {
		db.del(unw.added[i].input)
	}

	delete(u.blocksUnwind, height)
}


func (u *txUnwindData)Stats() (s string) {
	var ra, rd int
	for _, v := range u.blocksUnwind {
		ra += len(v.added)
		rd += len(v.deleted)
	}
	s += fmt.Sprintf("UNWIND  : blk_cnt=%d  adds=%d  dels=%d\n", 
		len(u.blocksUnwind), ra, rd)
	return
}


func (u *txUnwindData)Load(f *os.File) {
	u.blocksUnwind = make(map[uint32] *UnspentBlockUnwind, UnspentTxsMapInitLen)
	for {
		v := new(UnspentBlockUnwind)
		height, e := read32bit(f)
		if e != nil {
			break
		}

		le, _ := read32bit(f)
		v.added = make([]UnspentOneUnwind, le)
		for j := range v.added {
			v.added[j].Load(f)
		}
		
		le, _ = read32bit(f)
		v.deleted = make([]UnspentOneUnwind, le)
		for j := range v.deleted {
			v.deleted[j].Load(f)
		}

		u.blocksUnwind[height] = v
	}
	println(len(u.blocksUnwind), "loaded into unwindDB")
}


func (u *txUnwindData)Save(f *os.File) {
	for k, v := range u.blocksUnwind {
		write32bit(f, k)

		write32bit(f, uint32(len(v.added)))
		for j := range v.added {
			v.added[j].Save(f)
		}

		write32bit(f, uint32(len(v.deleted)))
		for j := range v.deleted {
			v.deleted[j].Save(f)
		}
	}
	println(len(u.blocksUnwind), "saved in unwindDB")
}

