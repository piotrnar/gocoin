package mysqldb

import (
	"os"
	"errors"
	"fmt"
//	"encoding/hex"
	"github.com/piotrnar/gocoin/btc"
	"github.com/ziutek/mymysql/mysql"
	_ "github.com/ziutek/mymysql/native"
)

const BlocksFilename = "c:\\blocks.bin"

type BtcDB struct {
	blks *os.File
	con mysql.Conn
	tr mysql.Transaction
	addunspentst, getunspentst, delunspentst mysql.Stmt
	addunwindst, delunwindst, selunwindst mysql.Stmt
	
	addblockst, updblockst, getblockst, posblockst mysql.Stmt
}

func NewDb() btc.BtcDB {
	var db BtcDB

	db.blks, _ = os.OpenFile(BlocksFilename, os.O_RDWR|os.O_APPEND|os.O_CREATE, 0660)

	c := mysql.New("tcp4", "", "127.0.0.1:3306", "go", "", "gocoin")
	e := c.Connect()
	if e != nil {
		return nil
	}
	db.con = c
	
	db.addunspentst, e = c.Prepare("INSERT INTO unspent(txid, vout, value, pkscr) VALUES(?, ?, ?, ?)")
	db.getunspentst, e = c.Prepare("SELECT value, pkscr FROM unspent WHERE txid=? AND vout=? LIMIT 1")
	db.delunspentst, e = c.Prepare("DELETE FROM unspent WHERE txid=? AND vout=? LIMIT 1")

	db.addunwindst, e = c.Prepare("INSERT INTO unwind(height, added, in_txid, in_vout, value, pkscr) VALUES(?, ?, ?, ?, ?, ?)")
	db.delunwindst, e = c.Prepare("DELETE FROM unwind WHERE height=?")
	db.selunwindst, e = c.Prepare("SELECT added, in_txid, in_vout, value, pkscr FROM unwind WHERE height=? ORDER by added")
	
	db.addblockst, e = c.Prepare("INSERT INTO blocks(height, hash, prev, fpos, len) VALUES(?, ?, ?, ?, ?)")
	db.updblockst, e = c.Prepare("UPDATE blocks SET orph=? WHERE hash=?")
	db.getblockst, e = c.Prepare("SELECT hash, prev, orph FROM blocks ORDER by height")
	db.posblockst, e = c.Prepare("SELECT fpos, `len` FROM blocks WHERE hash=? LIMIT 1")
	
	// Clean up the tables
	if false {
		c.Query("DELETE FROM unspent")
		c.Query("DELETE FROM unwind")
		c.Query("DELETE FROM blocks")
	}
	return &db
}


func (db BtcDB) UnspentPurge() {
	db.con.Query("DELETE FROM unspent")
	db.con.Query("DELETE FROM unwind")
	db.con.Query("UPDATE blocks SET orph=0")
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


func (db BtcDB) UnspentAdd(po *btc.TxPrevOut, rec *btc.TxOut) (e error) {
	_, e = db.addunspentst.Run(po.Hash[:], po.Vout, rec.Value, rec.Pk_script[:])
	return
}

func (db BtcDB) UnspentGet(po *btc.TxPrevOut) (res *btc.TxOut, e error) {
	var ro mysql.Row
	ro, _, e = db.getunspentst.ExecFirst(po.Hash[:], po.Vout)
	if e == nil && ro != nil {
		res = new(btc.TxOut)
		res.Value = ro.Uint64(0)
		res.Pk_script = ro.Bin(1)
	}
	return
}

func (db BtcDB) UnspentDel(po *btc.TxPrevOut) (e error) {
	_, e = db.delunspentst.Run(po.Hash[:], po.Vout)
	return
}


func (db BtcDB) UnwindAdd(height uint32, added int, po *btc.TxPrevOut, rec *btc.TxOut) (e error) {
	_, e = db.addunwindst.Run(height, added, po.Hash[:], po.Vout, rec.Value, rec.Pk_script[:])
	return
}

func (db BtcDB) UnwindDel(height uint32) (e error) {
	_, e = db.delunwindst.Run(height)
	return
}

func (db BtcDB) UnwindNow(height uint32) (e error) {
	var res mysql.Result
	res, e = db.selunwindst.Run(height)
	if e != nil {
		panic(e.Error())
	}
	
	var po btc.TxPrevOut
	var rec btc.TxOut
	for {
		ro, _ := res.GetRow()
		if ro == nil {
			break
		}
		copy(po.Hash[:], ro.Bin(1))
		po.Vout = uint32(ro.Uint(2))
		if ro.Int(0)==0 {
			// Deleted
			rec.Value = ro.Uint64(3)
			rec.Pk_script = ro.Bin(4)
			db.UnspentAdd(&po, &rec)
		} else {
			// Added
			db.UnspentDel(&po)
		}
	}
	_, e = db.delunwindst.Run(height)
	return
}

func (db BtcDB) GetStats() (s string) {
	rows, _, er := mysql.Query(db.con, 
		"SELECT COUNT(*), SUM(value), SUM(LENGTH(pkscr)) from unspent")
	if er==nil && len(rows)==1 {
		s += fmt.Sprintf("UNSPENT : tx_cnt=%d  btc=%.8f  size:%dMB\n", rows[0].Uint(0), 
			float64(rows[0].Uint64(1))/1e8, rows[0].Uint64(2)>>20)
	}

	rows, _, er = mysql.Query(db.con, 
		"SELECT COUNT(*), SUM(added) from unwind")
	if er==nil && len(rows)==1 {
		del := rows[0].Uint(0) - rows[0].Uint(1)
		s += fmt.Sprintf("UNWIND  : blk_cnt=%d  adds=%d  dels=%d\n", rows[0].Uint(0),
			rows[0].Uint(1), del)
	}
	
	rows, _, er = mysql.Query(db.con, 
		"SELECT MAX(height), COUNT(*), SUM(orph), SUM(`len`) from blocks")
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

func (db BtcDB) getuint(sql string) (res uint64, e error) {
	rows, _, er := mysql.Query(db.con, sql)
	if er != nil {
		e = er
		return
	}
	if len(rows)==0 {
		e = errors.New("No rows returned: "+sql)
	}
	res = rows[0].Uint64(0)
	return
}



func init() {
	btc.NewDb = NewDb
}

