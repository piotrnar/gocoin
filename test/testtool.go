package main

import (
	"os"
	"fmt"
	"github.com/piotrnar/gocoin/btc"
	"github.com/piotrnar/gocoin/btc/wierddb"
	"time"
	"flag"
	"bufio"
	"strconv"
	"github.com/piotrnar/gocoin/btc/blockdb"
)

var testnet *bool = flag.Bool("t", true, "use testnet")
var autoload *bool = flag.Bool("l", false, "auto load blocks")

var GenesisBlock *btc.Uint256
var Magic [4]byte
var BlockChain *btc.Chain
var BlockDatabase *blockdb.BlockDB

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
	var er error
	var dat []byte
	start := time.Now().UnixNano()
	var blkcnt, totbytes uint64
	for blkcnt=0; blkcnt<limit; blkcnt++ {
		dat, er = BlockDatabase.FetchNextBlock()
		if dat==nil || er!=nil {
			println("END of DB file")
			break
		}
		
		bl, er = btc.NewBlock(dat[:])
		if er != nil {
			println("Block inconsisrent:", er.Error())
			return
		}

		//println("got block len", len(bl.Raw))
		if GenesisBlock.Equal(bl.Hash) {
			fmt.Println("Skip genesis block")
			continue
		}

		totbytes += uint64(len(bl.Raw))
		er = bl.CheckBlock()
		errorFatal(er, "CheckBlock() failed")
		
		er = BlockChain.AcceptBlock(bl)
		if er != nil {
			//println("Block not accepted into chain:", er.Error())
		}
		
		if limit>10000 && blkcnt%10000 == 0 {
			printstat()
		}
	}
	stop := time.Now().UnixNano()
	fmt.Printf("Operation took: %.3fs, read %d blocks containing %dMB of data\n", 
		float64(stop-start)/1e9, blkcnt, totbytes>>20)
	printstat()
}


func askUserInt(ask string) int {
	ask = "Global commands: [I]nfo, [Q]uit\n" + ask + "\nEnter your choice: "
	for {
		fmt.Println("============================================")
		fmt.Print(ask)
		li, _, _ := sin.ReadLine()
		fmt.Println("............................................")
		n, e := strconv.ParseInt(string(li[:]), 10, 32)
		if e == nil {
			fmt.Println()
			return int(n)
		}
		switch string(li[:]) {
			case "i": printstat()
			case "q": os.Exit(0)
		}
	}
	return 0
}

func loadblocks_menu() {
	for {
		cmd := askUserInt("Enter number of block to scan, or 0 to exit:")
		if cmd == 0 {
			return
		}
		reloadBlockchain(uint64(cmd))
	}
}

func make_transaction() {
	fmt.Println()

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
	
	tx.TxIn = make([]*btc.TxIn, 1)
	tx.TxIn[0].Input = btc.TxPrevOut{Hash:intxid, Vout:intxco}
	//tx.ScriptSig = ???
	tx.TxIn[0].Sequence = 0
	
	tx.TxOut = make([]*btc.TxOut, 1)
	
	tx.TxOut[0].Value = unsp.Value // spend at all
	tx.TxOut[0].Pk_script = make([]byte, 25)
	tx.TxOut[0].Pk_script = []byte{0x76,0xa9,0x14,0x09,0xae,0xc0,0xbf,0x1f,0x5a,0xd5,0xed,0x22,0x09,0xd1,0x4c,0x42,0x84,0xc3,0xb9,0x90,0x9f,0x7b,0xae,0x88,0xac}
	
	tx.Lock_time = 0xffffffff

	println(bin2hex(tx.Unsigned()))
}


func list_unspent() {
	fmt.Print("Enter address to check: ")
	li, _, _ := sin.ReadLine()
	a, e := btc.NewAddrFromString(string(li[:]))
	if e != nil {
		println(e.Error())
		return
	}
	unsp := BlockChain.GetUnspentFromPkScr(a.OutScript())
	var sum uint64
	for i := range unsp {
		fmt.Println(unsp[i].String())
		sum += unsp[i].Value
	}
	fmt.Printf("Total %.8f unspent BTC at address %s\n", float64(sum)/1e8, a.Enc58str);
}


func menu_main() {
	for {
		cmd := askUserInt(" 1) load block\n 2) make transaction\n 3) list unspent")
		switch cmd {
			case 1: loadblocks_menu()
			case 2: make_transaction()
			case 3: list_unspent()
		}
	}
}


func main() {
	flag.Parse()

	var dir string
	
	if *testnet { // testnet3
		dir = os.Getenv("APPDATA")+"/Bitcoin/testnet3/blocks"
		GenesisBlock = btc.NewUint256FromString("000000000933ea01ad0ee984209779baaec3ced90fa3f408719526f8d77f4943")
		Magic = [4]byte{0x0B,0x11,0x09,0x07}
		wierddb.BlocksFilename = "c:\\testnet.bin"
	} else {
		dir = os.Getenv("APPDATA")+"/Bitcoin/blocks"
		GenesisBlock = btc.NewUint256FromString("000000000019d6689c085ae165831e934ff763ae46a2a6c172b3f1b60a8ce26f")
		Magic = [4]byte{0xF9,0xBE,0xB4,0xD9}
	}

	BlockDatabase = blockdb.NewBlockDB(dir, Magic)

	BlockChain = btc.NewChain(GenesisBlock)
	BlockChain.Rescan()
	//reloadBlockchain(1) // skip the genesis vlock
	
	menu_main()
}


func bin2hex(mem []byte) string {
	var s string
	for i := 0; i<len(mem); i++ {
		s+= fmt.Sprintf("%02x", mem[i])
	}
	return s
}

