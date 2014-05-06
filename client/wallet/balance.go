package wallet

import (
	"io"
	"os"
	"fmt"
	"sort"
	"sync"
	"io/ioutil"
	"encoding/hex"
	"encoding/binary"
	"github.com/piotrnar/gocoin/btc"
	"github.com/piotrnar/gocoin/client/common"
)

var (
	mutex_bal sync.Mutex
	MyBalance btc.AllUnspentTx  // unspent outputs that can be removed
	MyWallet *OneWallet     // addresses that cann be poped up
	LastBalance uint64
	BalanceChanged bool
	BalanceInvalid bool = true

	CachedAddrs map[[20]byte] *OneCachedAddrBalance = make(map[[20]byte] *OneCachedAddrBalance)
	CacheUnspent [] *OneCachedUnspent
	CacheUnspentIdx map[uint64] *OneCachedUnspentIdx = make(map[uint64] *OneCachedUnspentIdx)
)


type OneCachedUnspentIdx struct {
	Index uint
	Record *btc.OneUnspentTx
}


type OneCachedUnspent struct {
	*btc.BtcAddr
	btc.AllUnspentTx  // a cache for unspent outputs (from different wallets)
}

type OneCachedAddrBalance struct {
	InWallet bool
	CacheIndex uint
	Value uint64
}


func LockBal() {
	mutex_bal.Lock()
}

func UnlockBal() {
	mutex_bal.Unlock()
}


func po2idx(po *btc.TxPrevOut) uint64 {
	return binary.LittleEndian.Uint64(po.Hash[:8]) ^ uint64(po.Vout)
}


// This is called while accepting the block (from the chain's thread)
func TxNotify (idx *btc.TxPrevOut, valpk *btc.TxOut) {
	var update_wallet bool

	mutex_bal.Lock()
	defer mutex_bal.Unlock()

	if valpk!=nil {
		// Extract hash160 from pkscript
		adr := btc.NewAddrFromPkScript(valpk.Pk_script, common.Testnet)
		if adr==nil {
			return // We do not monitor this address
		}

		if rec, ok := CachedAddrs[adr.Hash160]; ok {
			rec.Value += valpk.Value
			utxo := new(btc.OneUnspentTx)
			utxo.TxPrevOut = *idx
			utxo.Value = valpk.Value
			utxo.MinedAt = valpk.BlockHeight
			utxo.BtcAddr = CacheUnspent[rec.CacheIndex].BtcAddr
			CacheUnspent[rec.CacheIndex].AllUnspentTx = append(CacheUnspent[rec.CacheIndex].AllUnspentTx, utxo)
			CacheUnspentIdx[po2idx(idx)] = &OneCachedUnspentIdx{Index: rec.CacheIndex, Record: utxo}
			if rec.InWallet {
				update_wallet = true
			}
		}
	} else {
		ii := po2idx(idx)
		if ab, present := CacheUnspentIdx[ii]; present {
			adrec := CacheUnspent[ab.Index]
			//println("removing", idx.String())
			rec := CachedAddrs[adrec.BtcAddr.Hash160]
			if rec==nil {
				panic("rec not found for " + adrec.BtcAddr.String())
			}
			rec.Value -= ab.Record.Value
			if rec.InWallet {
				update_wallet = true
			}
			for j := range adrec.AllUnspentTx {
				if adrec.AllUnspentTx[j] == ab.Record {
					//println("found it at index", j)
					adrec.AllUnspentTx = append(adrec.AllUnspentTx[:j], adrec.AllUnspentTx[j+1:]...)
					break
				}
			}
			delete(CacheUnspentIdx, ii)
		}
	}

	if MyWallet!=nil && update_wallet {
		MyBalance = nil
		for i := range MyWallet.Addrs {
			rec, _ := CachedAddrs[MyWallet.Addrs[i].Hash160]
			MyBalance = append(MyBalance, CacheUnspent[rec.CacheIndex].AllUnspentTx...)
		}
		sort_and_sum()
		BalanceChanged = true
	}
}


func GetRawTransaction(BlockHeight uint32, txid *btc.Uint256, txf io.Writer) bool {
	// Find the block with the indicated Height in the main tree
	common.BlockChain.BlockIndexAccess.Lock()
	n := common.Last.Block
	if n.Height < BlockHeight {
		println(n.Height, BlockHeight)
		common.BlockChain.BlockIndexAccess.Unlock()
		panic("This should not happen")
	}
	for n.Height > BlockHeight {
		n = n.Parent
	}
	common.BlockChain.BlockIndexAccess.Unlock()

	bd, _, e := common.BlockChain.Blocks.BlockGet(n.BlockHash)
	if e != nil {
		println("BlockGet", n.BlockHash.String(), BlockHeight, e.Error())
		println("This should not happen - please, report a bug.")
		println("You can probably fix it by launching the client with -rescan")
		os.Exit(1)
	}

	bl, e := btc.NewBlock(bd)
	if e != nil {
		println("NewBlock: ", e.Error())
		os.Exit(1)
	}

	e = bl.BuildTxList()
	if e != nil {
		println("BuildTxList:", e.Error())
		os.Exit(1)
	}

	// Find the transaction we need and store it in the file
	for i := range bl.Txs {
		if bl.Txs[i].Hash.Equal(txid) {
			txf.Write(bl.Txs[i].Serialize())
			return true
		}
	}
	return false
}


// Call it only from the Chain thread
func DumpBalance(mybal btc.AllUnspentTx, utxt *os.File, details, update_balance bool) (s string) {
	var sum uint64
	mutex_bal.Lock()
	defer mutex_bal.Unlock()

	for i := range mybal {
		sum += mybal[i].Value

		if details {
			if i<100 {
				s += fmt.Sprintf("%7d %s\n", 1+common.Last.Block.Height-mybal[i].MinedAt,
					mybal[i].String())
			} else if i==100 {
				s += fmt.Sprintln("List of unspent outputs truncated to 100 records")
			}
		}

		// update the balance/ folder
		if utxt != nil {
			po, e := common.BlockChain.Unspent.UnspentGet(&mybal[i].TxPrevOut)
			if e != nil {
				println("UnspentGet:", e.Error())
				println("This should not happen - please, report a bug.")
				println("You can probably fix it by launching the client with -rescan")
				os.Exit(1)
			}

			txid := btc.NewUint256(mybal[i].TxPrevOut.Hash[:])

			// Store the unspent line in balance/unspent.txt
			fmt.Fprintf(utxt, "%s # %.8f BTC @ %s, %d confs", mybal[i].TxPrevOut.String(),
				float64(mybal[i].Value)/1e8, mybal[i].BtcAddr.StringLab(),
				1+common.Last.Block.Height-mybal[i].MinedAt)
			if mybal[i].StealthC!=nil {
				fmt.Fprint(utxt, ", _StealthC:", hex.EncodeToString(mybal[i].StealthC))
			}
			fmt.Fprintln(utxt)

			// store the entire transactiojn in balance/<txid>.tx
			fn := "balance/"+txid.String()[:64]+".tx"
			txf, _ := os.Open(fn)
			if txf == nil {
				// Do it only once per txid
				txf, _ = os.Create(fn)
				if txf==nil {
					println("Cannot create ", fn)
					os.Exit(1)
				}
				GetRawTransaction(po.BlockHeight, txid, txf)
			}
			txf.Close()
		}
	}
	if update_balance {
		LastBalance = sum
	}
	s += fmt.Sprintf("Total balance: %.8f BTC in %d unspent outputs\n", float64(sum)/1e8, len(mybal))
	if utxt != nil {
		utxt.Close()
	}
	return
}


func UpdateBalance() {
	var tofetch []*btc.BtcAddr

	mutex_bal.Lock()
	defer mutex_bal.Unlock()

	MyBalance = nil

	for _, v := range CachedAddrs {
		v.InWallet = false
	}

	for i := range MyWallet.Addrs {
		if rec, pres := CachedAddrs[MyWallet.Addrs[i].Hash160]; pres {
			rec.InWallet = true
			for j := range CacheUnspent[rec.CacheIndex].AllUnspentTx {
				// update BtcAddr in each of AllUnspentTx to reflect the latest label
				CacheUnspent[rec.CacheIndex].AllUnspentTx[j].BtcAddr = MyWallet.Addrs[i]
			}
			MyBalance = append(MyBalance, CacheUnspent[rec.CacheIndex].AllUnspentTx...)
		} else {
			// Add a new address to the balance cache
			CachedAddrs[MyWallet.Addrs[i].Hash160] = &OneCachedAddrBalance{InWallet:true, CacheIndex:uint(len(CacheUnspent))}
			CacheUnspent = append(CacheUnspent, &OneCachedUnspent{BtcAddr:MyWallet.Addrs[i]})
			tofetch = append(tofetch, MyWallet.Addrs[i])
		}
	}

	if len(tofetch)>0 {
		//fmt.Println("Fetching a new blance for", len(tofetch))
		// There are new addresses which we have not monitored yet
		new_addrs := common.BlockChain.GetAllUnspent(tofetch, true)
		for i := range new_addrs {
			rec := CachedAddrs[new_addrs[i].BtcAddr.Hash160]
			rec.Value += new_addrs[i].Value
			CacheUnspent[rec.CacheIndex].AllUnspentTx = append(CacheUnspent[rec.CacheIndex].AllUnspentTx, new_addrs[i])
			CacheUnspentIdx[po2idx(&new_addrs[i].TxPrevOut)] = &OneCachedUnspentIdx{Index:rec.CacheIndex, Record:new_addrs[i]}
		}
		MyBalance = append(MyBalance, new_addrs...)
	}

	sort_and_sum()
}


// Calculate total balance and sort MyBalnace by block height
func sort_and_sum() {
	LastBalance = 0
	if len(MyBalance) > 0 {
		sort.Sort(MyBalance)
		for i := range MyBalance {
			LastBalance += MyBalance[i].Value
		}
	}
	BalanceInvalid = false
}


func UpdateBalanceFolder() string {
	os.RemoveAll("balance")
	os.MkdirAll("balance/", 0770)
	if BalanceInvalid {
		UpdateBalance()
	}
	utxt, _ := os.Create("balance/unspent.txt")
	return DumpBalance(MyBalance, utxt, true, false)
}

func LoadWallet(fn string) {
	MyWallet = NewWallet(fn)
	if MyWallet != nil {
		UpdateBalance()
	}
}

func FetchAllBalances() {
	dir := common.GocoinHomeDir + "wallet" + string(os.PathSeparator)
	fis, er := ioutil.ReadDir(dir)
	if er == nil {
		for i := range fis {
			if !fis[i].IsDir() && fis[i].Size()>1 {
				fpath := dir + fis[i].Name()
				//println("pre-cache wallet", fpath)
				if MyWallet==nil {
					LoadWallet(fpath)
				} else {
					tmp := NewWallet(fpath)
					for an := range tmp.Addrs {
						var fnd bool
						for ao := range MyWallet.Addrs {
							if MyWallet.Addrs[ao].Hash160==tmp.Addrs[an].Hash160 {
								fnd = true
								break
							}
						}
						if !fnd {
							MyWallet.Addrs = append(MyWallet.Addrs, tmp.Addrs[an])
						}
					}
				}
			}
		}
	}
	if MyWallet!=nil && len(MyWallet.Addrs)>0 {
		println("Fetching balance of", len(MyWallet.Addrs), "addresses")
		UpdateBalance()
		fmt.Printf("Total cached balance: %.8f BTC in %d unspent outputs\n", float64(LastBalance)/1e8, len(MyBalance))
	}
	PrecachingComplete = true
}
