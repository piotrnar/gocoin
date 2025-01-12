package wallet

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/piotrnar/gocoin/client/common"
	"github.com/piotrnar/gocoin/lib/btc"
)

const (
	CURRENT_FILE_VERSION = 1
	BALANCES_SUBDIR      = "bal"
)

var (
	LAST_SAVED_FNAME string
)

func (b *OneAllAddrBal) Save(key []byte, of io.Writer) {
	of.Write(key)
	btc.WriteVlen(of, btc.CompressAmount(b.Value))
	if b.unsp != nil {
		btc.WriteVlen(of, uint64(len(b.unsp)))
		for _, u := range b.unsp {
			of.Write(u[:])
		}
	} else if len(b.unspMap) != 0 {
		btc.WriteVlen(of, uint64(len(b.unspMap)))
		for k := range b.unspMap {
			of.Write(k[:])
		}
	} else {
		println("ERROR: OneAllAddrBal.Save() - this should not happen")
	}
}

func newAddrBal(rd io.Reader) (res *OneAllAddrBal) {
	b := new(OneAllAddrBal)
	le, er := btc.ReadVLen(rd)
	if er != nil {
		return
	}
	b.Value = btc.DecompressAmount(le)

	if le, er = btc.ReadVLen(rd); er != nil {
		return
	}
	if le == 0 {
		println("ERROR: newAddrBal - this should not happen")
		return
	}
	if int(le) >= common.CFG.AllBalances.UseMapCnt {
		var k OneAllAddrInp
		b.unspMap = make(map[OneAllAddrInp]bool, int(le))
		for ; le > 0; le-- {
			if _, er = io.ReadFull(rd, k[:]); er != nil {
				return
			}
			b.unspMap[k] = true
		}
	} else {
		b.unsp = make([]OneAllAddrInp, int(le))
		for i := range b.unsp {
			if _, er = io.ReadFull(rd, b.unsp[i][:]); er != nil {
				return
			}
		}
	}

	res = b
	return
}

func cur_fname() string {
	return fmt.Sprint(common.Last.Block.Height, "-",
		common.Last.Block.BlockHash.String()[64-10:64], "-",
		common.AllBalMinVal(), "-", CURRENT_FILE_VERSION, ".dmp")
}

func load_map20(rd io.Reader) (res map[[20]byte]*OneAllAddrBal, er error) {
	var le uint64
	var ke [20]byte
	if le, er = btc.ReadVLen(rd); er != nil {
		return
	}
	themap := make(map[[20]byte]*OneAllAddrBal, int(le))
	for ; le > 0; le-- {
		if _, er = io.ReadFull(rd, ke[:]); er != nil {
			return
		}
		themap[ke] = newAddrBal(rd)
	}
	res = themap
	return
}

func load_map32(rd io.Reader) (res map[[32]byte]*OneAllAddrBal, er error) {
	var le uint64
	var ke [32]byte
	if le, er = btc.ReadVLen(rd); er != nil {
		return
	}
	themap := make(map[[32]byte]*OneAllAddrBal, int(le))
	for ; le > 0; le-- {
		if _, er = io.ReadFull(rd, ke[:]); er != nil {
			return
		}
		themap[ke] = newAddrBal(rd)
	}
	res = themap
	return
}

func save_map20(wr io.Writer, tm map[[20]byte]*OneAllAddrBal) {
	btc.WriteVlen(wr, uint64(len(tm)))
	for k, rec := range tm {
		rec.Save(k[:], wr)
	}
}

func save_map32(wr io.Writer, tm map[[32]byte]*OneAllAddrBal) {
	btc.WriteVlen(wr, uint64(len(tm)))
	for k, rec := range tm {
		rec.Save(k[:], wr)
	}
}

func LoadBalances() (er error) {
	var _fi *os.File
	fname := cur_fname()
	if _fi, er = os.Open(common.GocoinHomeDir + BALANCES_SUBDIR + string(os.PathSeparator) + fname); er != nil {
		return
	}

	rd := bufio.NewReaderSize(_fi, 0x4000)

	if AllBalancesP2KH, er = load_map20(rd); er != nil {
		println("LoadBalances P2KH:", er.Error())
		return
	}
	if AllBalancesP2SH, er = load_map20(rd); er != nil {
		println("LoadBalances P2SH:", er.Error())
		return
	}
	if AllBalancesP2WKH, er = load_map20(rd); er != nil {
		println("LoadBalances P2WKH:", er.Error())
		return
	}
	if AllBalancesP2WSH, er = load_map32(rd); er != nil {
		println("LoadBalances P2WSH:", er.Error())
		return
	}
	if AllBalancesP2TAP, er = load_map32(rd); er != nil {
		println("LoadBalances P2TAP:", er.Error())
		return
	}

	_fi.Close()
	LAST_SAVED_FNAME = fname
	return

}

func SaveBalances() (er error) {
	if !common.GetBool(&common.WalletON) {
		er = errors.New("the wallet is not on")
		return
	}

	fname := cur_fname()
	if fname == LAST_SAVED_FNAME {
		er = errors.New(LAST_SAVED_FNAME + " already on disk")
		return
	}

	os.RemoveAll(common.GocoinHomeDir + BALANCES_SUBDIR)
	if !common.CFG.AllBalances.SaveBalances {
		er = errors.New("saving not requested")
		return
	}

	os.Mkdir(common.GocoinHomeDir+BALANCES_SUBDIR, 0770)
	fil, er := os.Create(common.GocoinHomeDir + "balances" + string(os.PathSeparator) + fname)
	if er != nil {
		return
	}

	of := bufio.NewWriterSize(fil, 0x100000)
	save_map20(of, AllBalancesP2KH)
	save_map20(of, AllBalancesP2SH)
	save_map20(of, AllBalancesP2WKH)
	save_map32(of, AllBalancesP2WSH)
	save_map32(of, AllBalancesP2TAP)
	of.Flush()
	fil.Close()
	LAST_SAVED_FNAME = fname
	return
}
