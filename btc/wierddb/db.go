package wierddb

import (
	"os"
	"errors"
	"fmt"
	"github.com/piotrnar/gocoin/btc"
	"github.com/ziutek/mymysql/mysql"
	_ "github.com/ziutek/mymysql/native"
)

var Testnet bool
var blocksFilename = "c:\\blocks.bin"
var blocksTable = "blocks"

type oneUnwind struct {
	prv *btc.TxPrevOut
	out *btc.TxOut
}

type oneUnwindSet struct {
	added []oneUnwind
	deled []oneUnwind
}


type BtcDB struct {
	blks *os.File
	con mysql.Conn

	mapUnspent map[[poutIdxLen]byte] *oneUnspent
	
	mapUnwind map[uint32] *oneUnwindSet

	addblockst, updblockst, getblockst, posblockst mysql.Stmt
}

func NewDb() btc.BtcDB {
	var db BtcDB

	db.blks, _ = os.OpenFile(blocksFilename, os.O_RDWR|os.O_APPEND|os.O_CREATE, 0660)

	c := mysql.New("tcp4", "", "127.0.0.1:3306", "go", "", "gocoin")
	e := c.Connect()
	if e != nil {
		return nil
	}
	db.con = c
	db.addblockst, e = c.Prepare("INSERT INTO "+blocksTable+"(height, hash, prev, fpos, len) VALUES(?, ?, ?, ?, ?)")
	db.updblockst, e = c.Prepare("UPDATE "+blocksTable+" SET orph=? WHERE hash=?")
	db.getblockst, e = c.Prepare("SELECT hash, prev, orph FROM "+blocksTable+" ORDER by height")
	db.posblockst, e = c.Prepare("SELECT fpos, `len` FROM "+blocksTable+" WHERE hash=? LIMIT 1")

	db.mapUnspent = make(map[[poutIdxLen]byte] *oneUnspent, 1000000)
	
	db.mapUnwind = make(map[uint32] *oneUnwindSet, 144)
	
	return &db
}


func (db BtcDB) StartTransaction() {
	db.con.Query("BEGIN")
}

func (db BtcDB) CommitTransaction() {
	db.con.Query("COMMIT")
}

func (db BtcDB) RollbackTransaction() {
	db.con.Query("ROLLBACK")
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

func (db BtcDB) GetStats() (s string) {
	sum := uint64(0)
	for _, v := range db.mapUnspent {
		sum += v.out.Value*uint64(v.cnt)
	}
	s += fmt.Sprintf("UNSPENT : tx_cnt=%d  tot_btc:%.8f\n", 
		len(db.mapUnspent), float64(sum)/1e8)
	
	s += fmt.Sprintf("UNWIND : blk_cnt=%d\n", len(db.mapUnwind))
	
	rows, _, er := mysql.Query(db.con, 
		"SELECT MAX(height), COUNT(*), SUM(orph), SUM(`len`) from "+blocksTable)
	if er==nil && len(rows)==1 {
		s += fmt.Sprintf("BCHAIN  : height=%d  OrphanedBlocks=%d/%d  : siz:~%dMB\n", 
			rows[0].Uint64(0), rows[0].Uint64(2), rows[0].Uint64(1), rows[0].Uint64(3)>>20)
	}

	return
}


func (db BtcDB) BlockAdd(height uint32, bl *btc.Block) (e error) {
	pos, _ := db.blks.Seek(0, os.SEEK_END)
	db.blks.Write(bl.Raw[:])
	//db.blks.Sync()
	_, e = db.addblockst.Run(height, bl.Hash.Hash[:], bl.GetParent()[:], pos, len(bl.Raw))
	return
}

func (db BtcDB) Close() {
	db.blks.Close()
}


func (db BtcDB) BlockGet(hash *btc.Uint256) (bl []byte, e error) {
	var ro mysql.Row
	ro, _, e = db.posblockst.ExecFirst(hash.Hash[:])
	if e != nil {
		println("dupa", e.Error())
		return
	}
	if ro != nil {
		db.blks.Seek(int64(ro.Uint64(0)), os.SEEK_SET)
		bl = make([]byte, ro.Uint64(1))
		_, e = db.blks.Read(bl[:])
	} else {
		e = errors.New("No such block in DB: " + hash.String())
		println("pipa", e.Error())
	}
	return
}

func (db BtcDB) BlockOrphan(hash *btc.Uint256, orph int) (e error) {
	_, e = db.updblockst.Run(orph, hash.Hash[:])
	return
}

func (db BtcDB) LoadBlockIndex(ch *btc.Chain, walk func(ch *btc.Chain, hash, prev []byte, orph int)) (e error) {
	var res mysql.Result
	res, e = db.getblockst.Run()
	if e != nil {
		panic(e.Error())
	}
	
	for {
		ro, _ := res.GetRow()
		if ro == nil {
			break
		}
		walk(ch, ro.Bin(0)[:], ro.Bin(1)[:], ro.Int(2))
	}
	return
}


func init() {
	btc.NewDb = NewDb
}

