package main

import (
	"os"
	"fmt"
	"sort"
	"github.com/piotrnar/gocoin/btc"
)

var (
	MyBalance btc.AllUnspentTx  // unspent outputs that can be removed
	MyWallet *oneWallet     // addresses that cann be poped up
	BalanceChanged bool
	BalanceInvalid bool = true
)

func TxNotify (idx *btc.TxPrevOut, valpk *btc.TxOut) {
	if valpk!=nil {
		for i := range MyWallet.addrs {
			if MyWallet.addrs[i].Owns(valpk.Pk_script) {
				if dbg>0 {
					fmt.Println(" +", idx.String(), valpk.String())
				}
				MyBalance = append(MyBalance, btc.OneUnspentTx{TxPrevOut:*idx,
					Value:valpk.Value, MinedAt:valpk.BlockHeight, BtcAddr:MyWallet.addrs[i]})
				BalanceChanged = true
				break
			}
		}
	} else {
		for i := range MyBalance {
			if MyBalance[i].TxPrevOut == *idx {
				tmp := make([]btc.OneUnspentTx, len(MyBalance)-1)
				if dbg>0 {
					fmt.Println(" -", MyBalance[i].String())
				}
				copy(tmp[:i], MyBalance[:i])
				copy(tmp[i:], MyBalance[i+1:])
				MyBalance = tmp
				BalanceChanged = true
				break
			}
		}
	}
}


func DumpBalance(utxt *os.File) {
	var sum uint64
	if len(MyBalance)>0 {
		sort.Sort(MyBalance)
	}
	for i := range MyBalance {
		sum += MyBalance[i].Value

		if len(MyBalance)<100 {
			fmt.Printf("%7d %s\n", 1+BlockChain.BlockTreeEnd.Height-MyBalance[i].MinedAt,
				MyBalance[i].String())
		}

		// update the balance/ folder
		if utxt != nil {
			po, e := BlockChain.Unspent.UnspentGet(&MyBalance[i].TxPrevOut)
			if e != nil {
				println("UnspentGet:", e.Error())
				fmt.Println("This should not happen - please, report a bug.")
				fmt.Println("You can probably fix it by launching the client with -rescan")
				os.Exit(1)
			}

			txid := btc.NewUint256(MyBalance[i].TxPrevOut.Hash[:])

			// Store the unspent line in balance/unspent.txt
			fmt.Fprintf(utxt, "%s # %.8f BTC @ %s, %d confs\n", MyBalance[i].TxPrevOut.String(),
				float64(MyBalance[i].Value)/1e8, MyBalance[i].BtcAddr.String(),
				1+BlockChain.BlockTreeEnd.Height-MyBalance[i].MinedAt)

			// store the entire transactiojn in balance/<txid>.tx
			fn := "balance/"+txid.String()[:64]+".tx"
			txf, _ := os.Open(fn)
			if txf != nil {
				// This file already exist - do no need to redo it
				txf.Close()
				continue
			}

			// Find the block with the indicated Height in the main tree
			BlockChain.BlockIndexAccess.Lock()
			n := BlockChain.BlockTreeEnd
			if n.Height < po.BlockHeight {
				println(n.Height, po.BlockHeight)
				BlockChain.BlockIndexAccess.Unlock()
				panic("This should not happen")
			}
			for n.Height > po.BlockHeight {
				n = n.Parent
			}
			BlockChain.BlockIndexAccess.Unlock()

			bd, _, e := BlockChain.Blocks.BlockGet(n.BlockHash)
			if e != nil {
				println("BlockGet", n.BlockHash.String(), po.BlockHeight, e.Error())
				fmt.Println("This should not happen - please, report a bug.")
				fmt.Println("You can probably fix it by launching the client with -rescan")
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
					txf, _ = os.Create(fn)
					if txf==nil {
						println("Cannot create ", fn)
						os.Exit(1)
					}
					txf.Write(bl.Txs[i].Serialize())
					txf.Close()
					break
				}
			}
		}
	}
	fmt.Printf("%.8f BTC in total, in %d unspent outputs\n", float64(sum)/1e8, len(MyBalance))
	if utxt != nil {
		fmt.Println("Your balance data has been saved to the 'balance/' folder.")
		fmt.Println("You nend to move this folder to your wallet PC, to spend the coins.")
		utxt.Close()
	}
}


func show_balance(p string) {
	if p!="" {
		fmt.Println("Using wallet from file", p, "...")
		LoadWallet(p)
	}

	if MyWallet==nil {
		println("You have no loaded wallet")
		return
	}

	if len(MyWallet.addrs)==0 {
		println("Your loaded wallet has no addresses")
		return
	}
	os.RemoveAll("balance")
	os.MkdirAll("balance/", 0770)

	if BalanceInvalid {
		MyBalance = BlockChain.GetAllUnspent(MyWallet.addrs, true)
		BalanceInvalid = false
	}

	utxt, _ := os.Create("balance/unspent.txt")
	DumpBalance(utxt)
}


func init() {
	newUi("balance bal", true, show_balance, "Show & save balance of currently loaded or a specified wallet")
}
