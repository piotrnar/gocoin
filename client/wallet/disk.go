package wallet

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"

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
		common.AllBalMinVal(), "-", CURRENT_FILE_VERSION)
}

func load_map20(fn string, res *map[[20]byte]*OneAllAddrBal, wg *sync.WaitGroup) {
	var ke [20]byte
	var le uint64
	var er error

	defer wg.Done()
	if f, _ := os.Open(fn); f != nil {
		rd := bufio.NewReaderSize(f, 0x4000)
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
		*res = themap
	}
}

func load_map32(fn string, res *map[[32]byte]*OneAllAddrBal, wg *sync.WaitGroup) {
	var ke [32]byte
	var le uint64
	var er error

	defer wg.Done()
	if f, _ := os.Open(fn); f != nil {
		rd := bufio.NewReaderSize(f, 0x4000)
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
		*res = themap
	}
}

func save_map20(fn string, tm map[[20]byte]*OneAllAddrBal, wg *sync.WaitGroup) {
	if f, _ := os.Create(fn); f != nil {
		wr := bufio.NewWriterSize(f, 0x100000)
		btc.WriteVlen(wr, uint64(len(tm)))
		for k, rec := range tm {
			rec.Save(k[:], wr)
		}
		wr.Flush()
		f.Close()
	}
	wg.Done()
}

func save_map32(fn string, tm map[[32]byte]*OneAllAddrBal, wg *sync.WaitGroup) {
	if f, _ := os.Create(fn); f != nil {
		wr := bufio.NewWriterSize(f, 0x40000)
		btc.WriteVlen(wr, uint64(len(tm)))
		for k, rec := range tm {
			rec.Save(k[:], wr)
		}
		wr.Flush()
		f.Close()
	}
	wg.Done()
}

func LoadBalances() (er error) {
	fname := cur_fname()
	dir := common.GocoinHomeDir + BALANCES_SUBDIR + string(os.PathSeparator) + fname
	if _, er = os.Stat(dir); os.IsNotExist(er) {
		er = errors.New(dir + " does not exist")
		return
	}

	dir += string(os.PathSeparator)

	var wg sync.WaitGroup
	wg.Add(5)
	go load_map20(dir+"P2KH", &AllBalancesP2KH, &wg)
	go load_map20(dir+"P2SH", &AllBalancesP2SH, &wg)
	go load_map20(dir+"P2WKH", &AllBalancesP2WKH, &wg)
	go load_map32(dir+"P2WSH", &AllBalancesP2WSH, &wg)
	go load_map32(dir+"P2TAP", &AllBalancesP2TAP, &wg)
	wg.Wait()

	println(AllBalancesP2KH, AllBalancesP2SH, AllBalancesP2WKH, AllBalancesP2WSH, AllBalancesP2TAP)

	if AllBalancesP2KH == nil || AllBalancesP2SH == nil || AllBalancesP2WKH == nil ||
		AllBalancesP2WSH == nil || AllBalancesP2TAP == nil {
		er = errors.New("balances could not be restored from " + dir)
	}

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

	dir := common.GocoinHomeDir + BALANCES_SUBDIR
	os.RemoveAll(dir)
	if !common.CFG.AllBalances.SaveBalances {
		er = errors.New("saving not requested")
		return
	}

	dir += string(os.PathSeparator) + fname
	if er = os.MkdirAll(dir, 0770); er != nil {
		return
	}
	dir += string(os.PathSeparator)

	var wg sync.WaitGroup
	wg.Add(5)
	go save_map20(dir+"P2KH", AllBalancesP2KH, &wg)
	go save_map20(dir+"P2SH", AllBalancesP2SH, &wg)
	go save_map20(dir+"P2WKH", AllBalancesP2WKH, &wg)
	go save_map32(dir+"P2WSH", AllBalancesP2WSH, &wg)
	go save_map32(dir+"P2TAP", AllBalancesP2TAP, &wg)
	wg.Wait()
	LAST_SAVED_FNAME = fname
	return
}
