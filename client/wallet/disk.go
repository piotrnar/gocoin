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
	"github.com/piotrnar/gocoin/lib/utxo"
)

const (
	CURRENT_FILE_VERSION = 2
	BALANCES_SUBDIR      = "bal"
)

var (
	LAST_SAVED_FNAME string
)

func dump_folder_name() string {
	return fmt.Sprint(common.Last.Block.Height, "-",
		common.Last.Block.BlockHash.String()[64-8:64], "-",
		utxo.UtxoIdxLen, "-", common.AllBalMinVal(), "-",
		CURRENT_FILE_VERSION)
}

func (b *OneAllAddrBal) Save(key []byte, of *bufio.Writer) {
	of.Write(key)
	btc.WriteVarInt(of, btc.CompressAmount(b.Value))
	if b.unsp != nil {
		btc.WriteVarInt(of, uint64(len(b.unsp)))
		for _, u := range b.unsp {
			of.Write(u[:])
		}
	} else if len(b.unspMap) != 0 {
		btc.WriteVarInt(of, uint64(len(b.unspMap)))
		for k := range b.unspMap {
			of.Write(k[:])
		}
	} else {
		println("ERROR: OneAllAddrBal.Save() - this should not happen")
	}
}

func newAddrBal(rd *bufio.Reader) (res *OneAllAddrBal) {
	b := new(OneAllAddrBal)
	le, er := btc.ReadVarInt(rd)
	if er != nil {
		return
	}
	b.Value = btc.DecompressAmount(le)

	if le, er = btc.ReadVarInt(rd); er != nil {
		return
	}
	if le == 0 {
		println("ERROR: newAddrBal - this should not happen")
		return
	}
	if int(le) >= useMapCnt {
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

func load_map(dir string, idx int, wg *sync.WaitGroup) {
	var le uint64
	var er error

	defer wg.Done()
	ke := make([]byte, IDX2SIZE[idx])
	if f, _ := os.Open(dir + IDX2SYMB[idx]); f != nil {
		rd := bufio.NewReaderSize(f, 0x4000)
		if le, er = btc.ReadVLen(rd); er != nil {
			return
		}
		themap := make(map[string]*OneAllAddrBal, int(le))
		for ; le > 0; le-- {
			if _, er = io.ReadFull(rd, ke); er != nil {
				return
			}
			themap[string(ke)] = newAddrBal(rd)
		}
		f.Close()
		allBalances[idx] = themap
	}
}

func save_map(fn string, tm map[string]*OneAllAddrBal, wg *sync.WaitGroup) {
	if f, _ := os.Create(fn); f != nil {
		wr := bufio.NewWriterSize(f, 0x100000)
		btc.WriteVlen(wr, uint64(len(tm)))
		for k, rec := range tm {
			rec.Save([]byte(k), wr)
		}
		wr.Flush()
		f.Close()
	}
	wg.Done()
}

func LoadBalances() (er error) {
	fname := dump_folder_name()
	dir := common.GocoinHomeDir + BALANCES_SUBDIR + string(os.PathSeparator) + fname
	if _, er = os.Stat(dir); os.IsNotExist(er) {
		er = errors.New(dir + " does not exist")
		return
	}

	dir += string(os.PathSeparator)

	var wg sync.WaitGroup
	for i := range allBalances {
		wg.Add(1)
		go load_map(dir, i, &wg)
	}
	wg.Wait()

	for i := range allBalances {
		if allBalances[i] == nil {
			er = errors.New(IDX2SYMB[i] + " balances could not be restored from " + dir)
			return
		}
	}

	common.BlockChain.Unspent.CB.NotifyTxAdd = TxNotifyAdd
	common.BlockChain.Unspent.CB.NotifyTxDel = TxNotifyDel
	common.Set(&common.WalletON, true)
	common.Set(&common.WalletOnIn, 0)
	LAST_SAVED_FNAME = fname
	return

}

func SaveBalances() (er error) {
	if !common.Get(&common.WalletON) {
		er = errors.New("the wallet is not on")
		return
	}

	fname := dump_folder_name()
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
	for i := range allBalances {
		wg.Add(1)
		go save_map(dir+IDX2SYMB[i], allBalances[i], &wg)
	}
	wg.Wait()
	LAST_SAVED_FNAME = fname
	return
}
