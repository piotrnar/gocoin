package wallet

import (
	"bufio"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/piotrnar/gocoin/client/common"
	"github.com/piotrnar/gocoin/lib/btc"
	"github.com/piotrnar/gocoin/lib/utxo"
)

const (
	CURRENT_FILE_VERSION = 4
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

func (b *OneAllAddrBal) Save(key OneAddrIndex, of *bufio.Writer) {
	binary.Write(of, binary.LittleEndian, key)
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
		b.unspMap = make(map[OneAllAddrInp]struct{}, int(le))
		for ; le > 0; le-- {
			if _, er = io.ReadFull(rd, k[:]); er != nil {
				return
			}
			b.unspMap[k] = struct{}{}
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

func load_map(dir string) {
	var le uint64
	var er error

	if f, _ := os.Open(dir + "ALL"); f != nil {
		rd := bufio.NewReaderSize(f, 0x4000)
		if le, er = btc.ReadVLen(rd); er != nil {
			return
		}
		themap := make(map[OneAddrIndex]*OneAllAddrBal, int(le))
		var ke OneAddrIndex
		for ; le > 0; le-- {
			if er = binary.Read(rd, binary.LittleEndian, &ke); er != nil {
				return
			}
			themap[ke] = newAddrBal(rd)
		}
		f.Close()
		allBalances = themap
	}
}

func save_map(fn string, tm map[OneAddrIndex]*OneAllAddrBal) {
	if f, _ := os.Create(fn); f != nil {
		wr := bufio.NewWriterSize(f, 0x100000)
		btc.WriteVlen(wr, uint64(len(tm)))
		for k, rec := range tm {
			rec.Save(k, wr)
		}
		wr.Flush()
		f.Close()
	}
}

func LoadBalances() (er error) {
	useMapCnt = int(common.Get(&common.CFG.AllBalances.UseMapCnt))

	fname := dump_folder_name()
	dir := common.GocoinHomeDir + BALANCES_SUBDIR + string(os.PathSeparator) + fname
	if _, er = os.Stat(dir); os.IsNotExist(er) {
		er = errors.New(dir + " does not exist")
		return
	}

	dir += string(os.PathSeparator)

	load_map(dir)

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

	save_map(dir+"ALL", allBalances)
	LAST_SAVED_FNAME = fname
	return
}
