package main

import (
	"os"
	"fmt"
	"piotr/btc"
	"time"
	"flag"
	"bufio"
	"strconv"
)

var testnet *bool = flag.Bool("t", false, "use testnet")
var autoload *bool = flag.Bool("l", false, "auto load blocks")

var GenesisBlock *btc.Uint256
var Magic [4]byte
var BlockChain *btc.Chain
var BlockDatabase *btc.BlockDB

var sin = bufio.NewReader(os.Stdin)

func errorFatal(er error, s string) {
	if er != nil {
		println(s+":", er.Error())
		os.Exit(1)
	}
}


func printstat() {
	fmt.Println(BlockChain.Stats())
}


func reloadBlockchain(limit uint64) {
	var bl *btc.Block
	start := time.Now().UnixNano()
	var blkcnt, totbytes uint64
	for blkcnt=0; blkcnt<limit; blkcnt++ {
		b, er := BlockDatabase.ReadNextBlock()
		if er != nil {
			//println(er.Error())
			break
		}
		totbytes += uint64(len(b))
		
		bl, er = btc.NewBlock(b)
		if GenesisBlock.Equal(bl.Hash) {
			fmt.Println("Skip genesis block")
			continue
		}
		errorFatal(er, "btc.NewBlock")

		er = bl.CheckBlock()
		errorFatal(er, "CheckBlock() failed")
		
		er = BlockChain.AcceptBlock(bl)
		if er != nil {
			println("Block not accepted into chain:", er.Error())
		}
		
		if limit>10000 && blkcnt%10000 == 0 {
			printstat()
		}
		
		/*
		if BlockChain.GetHeight()==61934 {
			println("dbg-on")
			btc.DbgSwitch(btc.DBG_BLOCKS, true)
		}
		*/

		/*
		h := BlockChain.GetHeight()
		if h>90000 {
			if BlockChain.TotalUnspent() != 50e8*uint64(h) {
				fmt.Printf("Wykurw na %d\n", h)
				break
			}
		}
		*/
	}
	stop := time.Now().UnixNano()
	fmt.Printf("Operation took: %.3fs, read %d blocks containing %dMB of data\n", 
		float64(stop-start)/1e9, blkcnt, totbytes>>20)
	printstat()
}


func askUserInt(ask string) int {
	fmt.Print(ask)
	for {
		li, _, _ := sin.ReadLine()
		n, e := strconv.ParseInt(string(li[:]), 10, 32)
		if e == nil {
			fmt.Println()
			return int(n)
		}
	}
	return 0
}

func loadblocks_menu() {
loop:
	cmd := askUserInt(`
 1) scan 1 block
 2) scan 10 block
 3) scan 100 block
 4) scan 1000 block
 5) scan 10000 block
 6) scan 91840 block
 7) scan 100000 block
 9) print stats
 0) Quit
Enter number:`)

	switch cmd {
		case 1: reloadBlockchain(1)
		case 2: reloadBlockchain(10)
		case 3: reloadBlockchain(100)
		case 4: reloadBlockchain(1000)
		case 5: reloadBlockchain(10000)
		case 6: reloadBlockchain(91840)
		case 7: reloadBlockchain(100000)
		case 9: printstat()
		case 0: return
	}
	goto loop
}



func make_transaction() {
/*
{
"txid" : "fafcb1ecf8ebf87e8cc75ab7dd37536d4c24f025b5142ba82d1a39e10db9ac12",
"vout" : 5,
"scriptPubKey" : "76a91409aec0bf1f5ad5ed2209d14c4284c3b9909f7bae88ac",
"amount" : 19.47650000,
"confirmations" : 463
}
*/
	fmt.Println()

	//intxid := btc.NewUint256FromString("fafcb1ecf8ebf87e8cc75ab7dd37536d4c24f025b5142ba82d1a39e10db9ac12")
	intxid := btc.NewUint256FromString("44ec33ba11a1199b808dd1b8f20d7caeee3ec0c7de2139a367c6a01a16dfa6f1").Hash
	intxco := uint32(0)

	unsp := BlockChain.LookUnspent(intxid, intxco)
	if unsp==nil {
		fmt.Println("no such unpent")
		return
	}
	fmt.Printf("%.8f BTC to spend\n", float64(unsp.Value)/1e8)
	fmt.Println()
	
	var tx btc.Tx
	
	tx.Version = 1
	
	tx.TxIn = make([]btc.TxIn, 1)
	tx.TxIn[0].Input = btc.TxPrevOut{Hash:intxid, Index:intxco}
	//tx.ScriptSig = ???
	tx.TxIn[0].Sequence = 0
	
	tx.TxOut = make([]btc.TxOut, 1)
	
	tx.TxOut[0].Value = unsp.Value // spend at all
	tx.TxOut[0].Pk_script = make([]byte, 25)
	tx.TxOut[0].Pk_script = []byte{0x76,0xa9,0x14,0x09,0xae,0xc0,0xbf,0x1f,0x5a,0xd5,0xed,0x22,0x09,0xd1,0x4c,0x42,0x84,0xc3,0xb9,0x90,0x9f,0x7b,0xae,0x88,0xac}
	
	tx.Lock_time = 0xffffffff

	println(bin2hex(tx.Unsigned()))

/*
unsigned:
0100000001f1a6df161aa0c667a33921dec7c03eeeae7c0df2b8d18d809b19a111ba33ec4400000000000000000001d0c71674000000001976a91409aec0bf1f5ad5ed2209d14c4284c3b9909f7bae88acffffffff

signed:
0100000001f1a6df161aa0c667a33921dec7c03eeeae7c0df2b8d18d809b19a111ba33ec44000000006b48304502201f8620292fecb7cf8d33ebf0f9115e95d4752533335d7201e08d9d940a87f442022100d8ff799b1002158215f830b1e3ade85497d73348e3ea2c809231d2a2dd3bb80b012102f0251d3bc8242123ec1feca1e54efc6bf984dfbb63b3fe0ae599f51f6037f0ea0000000001d0c71674000000001976a91409aec0bf1f5ad5ed2209d14c4284c3b9909f7bae88acffffffff
*/
}


func menu_main() {
loop:
	cmd := askUserInt(`
 1) load block
 2) make transaction
 3) save blockchain status
 9) print stats
 0) Quit
Enter number:`)

	switch cmd {
		case 1: loadblocks_menu()
		case 2: make_transaction()
		case 3: BlockChain.Save()
		case 9: printstat()
		case 0: return
	}
	goto loop
}


func main() {
	flag.Parse()

	var dir string
	
	if *testnet { // testnet3
		//dir = os.Getenv("APPDATA")+"/Bitcoin/blocks/testnet3/blocks"
		dir = "testnet3/blocks"
		GenesisBlock = btc.NewUint256FromString("000000000933ea01ad0ee984209779baaec3ced90fa3f408719526f8d77f4943")
		Magic = [4]byte{0x0B,0x11,0x09,0x07}
	} else {
		dir = os.Getenv("APPDATA")+"/Bitcoin/blocks"
		GenesisBlock = btc.NewUint256FromString("000000000019d6689c085ae165831e934ff763ae46a2a6c172b3f1b60a8ce26f")
		Magic = [4]byte{0xF9,0xBE,0xB4,0xD9}
	}

	BlockDatabase = btc.NewBlockDB(dir, Magic)
	BlockChain = btc.NewChain(BlockDatabase, GenesisBlock)
	if *autoload {
		reloadBlockchain(1+177778)
	} else {
		reloadBlockchain(1)
	}
	menu_main()
}


func bin2hex(mem []byte) string {
	var s string
	for i := 0; i<len(mem); i++ {
		s+= fmt.Sprintf("%02x", mem[i])
	}
	return s
}

