package wallet

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"unsafe"

	"github.com/piotrnar/gocoin/client/common"
	"github.com/piotrnar/gocoin/lib/utxo"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
)

var (
	FetchingBalanceTick func() bool
	OnOff               chan bool = make(chan bool, 1)
)

func update_meta() {
	common.Last.Mutex.Lock()
	baldb.Put([]byte("LBH"), common.Last.Block.BlockHash.Hash[:], nil)
	var tmp [8]byte
	binary.LittleEndian.PutUint64(tmp[:], common.Get(&common.CFG.AllBalances.MinValue))
	baldb.Put([]byte("MIN"), tmp[:], nil)
	baldb.Put([]byte("ILE"), []byte{utxo.UtxoIdxLen}, nil)
	common.Last.Mutex.Unlock()
}

func SaveBalances() (er error) {
	if !common.Get(&common.WalletON) {
		er = errors.New("the wallet is not on")
		return
	}

	if baldb != nil {
		update_meta()
		baldb.Close()
	}
	return
}

func ldb_dir() string {
	return common.GocoinHomeDir + "wallet.ldb"
}

// this should initiate/empty levelbd database
func CreateDB() (er error) {
	dir := ldb_dir()
	os.RemoveAll(dir)
	baldb, er = leveldb.OpenFile(dir, &opt.Options{NoSync: true})
	return
}

// this should initiate/empty levelbd database
func OpenDB() (er error) {
	var lbh []byte
	var hash_ok bool

	defer func() {
		if er != nil && baldb != nil {
			baldb.Close()
			baldb = nil
		}
	}()

	baldb, er = leveldb.OpenFile(ldb_dir(), &opt.Options{ErrorIfMissing: true})
	if er != nil {
		return
	}

	if lbh, er = baldb.Get([]byte("ILE"), nil); er != nil || len(lbh) != 1 {
		er = errors.New("wallet.ldb does not have a valid ILE record")
		return
	}
	if lbh[0] != utxo.UtxoIdxLen {
		er = errors.New("wallet.ldb is for a different UtxoIdxLen")
		return
	}

	if lbh, er = baldb.Get([]byte("MIN"), nil); er != nil || len(lbh) != 8 {
		er = errors.New("wallet.ldb does not have a valid MIN record")
		return
	}
	if binary.LittleEndian.Uint64(lbh) != common.Get(&common.CFG.AllBalances.MinValue) {
		er = errors.New("wallet.ldb is for a different MinValue")
		return
	}

	if lbh, er = baldb.Get([]byte("LBH"), nil); er != nil || len(lbh) != 32 {
		er = errors.New("wallet.ldb does not have a valid LBH record")
		return
	}

	common.Last.Mutex.Lock()
	hash_ok = bytes.Equal(lbh, common.Last.Block.BlockHash.Hash[:])
	common.Last.Mutex.Unlock()

	if !hash_ok {
		er = errors.New("wallet.ldb is for a different last block hash")
		return
	}
	return
}

func LoadBalancesFromUtxo() {
	if common.Get(&common.WalletON) {
		//fmt.Println("wallet.LoadBalance() ignore: ", common.GetBool(&common.WalletON))
		return
	}

	common.Set(&common.WalletProgress, 1)

	if er := OpenDB(); er == nil {
		fmt.Println("Successfully open previous wallet.ldb")
		return
	} else {
		fmt.Println(er.Error())
	}

	isFetching = make(map[string]*OneAllAddrBal, 30e6)

	var prv_key = -1
	var aborted bool
	common.BlockChain.Unspent.Browse(func(k, v []byte) bool {
		TxNotifyAdd(utxo.NewUtxoRecStatic(*(*utxo.UtxoKeyType)(unsafe.Pointer(&k[0])), v))
		if FetchingBalanceTick != nil && FetchingBalanceTick() {
			aborted = true
			return true
		}
		if int(k[0]) != prv_key {
			prv_key = int(k[0])
			common.Set(&common.WalletProgress, uint32(500*prv_key/256))
		}
		return aborted
	})

	if !aborted {
		if er := CreateDB(); er != nil {
			fmt.Println("Error creating a fresh database:", er.Error())
			aborted = true
		} else {
			var cnt int
			one_tick := len(isFetching) / 500
			perc := uint32(500)
			for k, r := range isFetching {
				baldb.Put([]byte(k), r.Serialize(), nil)
				if cnt == 0 {
					if FetchingBalanceTick != nil && FetchingBalanceTick() {
						aborted = true
						break
					}
					common.Set(&common.WalletProgress, perc)
					perc++
					cnt = one_tick
				} else {
					cnt--
				}
			}
			common.ApplyBalMinVal()
			update_meta()
			baldb.Close()
			baldb = nil
			fmt.Println("Fetching balance done. Now reopen database in sync mode.")
			if er := OpenDB(); er != nil {
				println(er.Error())
				aborted = true
			}
		}
	}
	isFetching = nil

	if aborted {
		Disable()
	} else {
		common.BlockChain.Unspent.CB.NotifyTxAdd = TxNotifyAdd
		common.BlockChain.Unspent.CB.NotifyTxDel = TxNotifyDel
		common.Set(&common.WalletON, true)
	}
	common.Set(&common.WalletProgress, 0)
}

func Disable() {
	if baldb != nil {
		baldb.Close()
		baldb = nil
	}
	if !common.Get(&common.WalletON) {
		//fmt.Println("wallet.Disable() ignore: ", common.GetBool(&common.WalletON))
		return
	}
	common.BlockChain.Unspent.CB.NotifyTxAdd = nil
	common.BlockChain.Unspent.CB.NotifyTxDel = nil
	common.Set(&common.WalletON, false)
}
