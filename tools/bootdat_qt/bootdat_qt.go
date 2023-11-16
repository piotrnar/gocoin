// This tool can import blockchain database from satoshi client to gocoin
package main

import (
	"os"
	"fmt"
	//"time"
	"encoding/hex"
	"encoding/binary"
	"github.com/piotrnar/gocoin/lib/btc"
	"github.com/piotrnar/gocoin/lib/chain"
	//"github.com/piotrnar/gocoin/lib/others/blockdb"
	//"github.com/piotrnar/gocoin/lib/others/sys"
)

const (
	GenesisBitcoin = "0100000000000000000000000000000000000000000000000000000000000000000000003ba3edfd7a7b12b27ac72c3e67768f617fc81bc3888a51323a9fb8aa4b1e5e4a29ab5f49ffff001d1dac2b7c0101000000010000000000000000000000000000000000000000000000000000000000000000ffffffff4d04ffff001d0104455468652054696d65732030332f4a616e2f32303039204368616e63656c6c6f72206f6e206272696e6b206f66207365636f6e64206261696c6f757420666f722062616e6b73ffffffff0100f2052a01000000434104678afdb0fe5548271967f1a67130b7105cd6a828e03909a67962e0ea1f61deb649f6bc3f4cef38c4f35504e51ec112de5c384df7ba0b8d578a4c702b6bf11d5fac00000000"
	GenesisTestnet = "0100000000000000000000000000000000000000000000000000000000000000000000003ba3edfd7a7b12b27ac72c3e67768f617fc81bc3888a51323a9fb8aa4b1e5e4adae5494dffff001d1aa4ae180101000000010000000000000000000000000000000000000000000000000000000000000000ffffffff4d04ffff001d0104455468652054696d65732030332f4a616e2f32303039204368616e63656c6c6f72206f6e206272696e6b206f66207365636f6e64206261696c6f757420666f722062616e6b73ffffffff0100f2052a01000000434104678afdb0fe5548271967f1a67130b7105cd6a828e03909a67962e0ea1f61deb649f6bc3f4cef38c4f35504e51ec112de5c384df7ba0b8d578a4c702b6bf11d5fac00000000"
)


var (
	bidx map[[32]byte] *chain.BlockTreeNode
	cnt int
)


func walk(ch *chain.Chain, hash, hdr []byte, height, blen, txs uint32) {
	bh := btc.NewUint256(hash)
	if _, ok := bidx[bh.Hash]; ok {
		println("walk: ", bh.String(), "already in")
		return
	}
	v := new(chain.BlockTreeNode)
	v.BlockHash = bh
	v.Height = height
	v.BlockSize = blen
	v.TxCount = txs
	copy(v.BlockHeader[:], hdr)
	bidx[bh.Hash] = v
	cnt++
}


func main() {
	if len(os.Args)<2 {
		fmt.Println("Specify a path to folder containig blockchain.dat and blockchain.new")
		fmt.Println("Output bootstrap.dat file will be written in the current folder.")
		return
	}

	blks := chain.NewBlockDB(os.Args[1])
	if blks==nil {
		return
	}
	fmt.Println("Loading block index...")
	bidx = make(map[[32]byte] *chain.BlockTreeNode, 300e3)
	blks.LoadBlockIndex(nil, walk)

	var tail, nd *chain.BlockTreeNode
	var genesis_block_hash *btc.Uint256
	for _, v := range bidx {
		if v==tail {
			// skip root block (should be only one)
			continue
		}

		par_hash := btc.NewUint256(v.BlockHeader[4:36])
		par, ok := bidx[par_hash.Hash]
		if !ok {
			genesis_block_hash = par_hash
		} else {
			v.Parent = par
			if tail==nil || v.Height > tail.Height {
				tail = v
			}
		}
	}

	if genesis_block_hash == nil {
		println("genesis_block_hash not found")
		return
	}

	var magic []byte

	gen_bin, _ := hex.DecodeString(GenesisBitcoin)
	tmp := btc.NewSha2Hash(gen_bin[:80])
	if genesis_block_hash.Equal(tmp) {
		println("Bitcoin genesis block")
		magic = []byte{0xF9,0xBE,0xB4,0xD9}
	}


	if magic==nil {
		gen_bin, _ := hex.DecodeString(GenesisTestnet)
		tmp = btc.NewSha2Hash(gen_bin[:80])
		if genesis_block_hash.Equal(tmp) {
			println("Testnet3 genesis block")
			magic = []byte{0x0B,0x11,0x09,0x07}
		}
	}

	if magic==nil {
		println("Unknow genesis block", genesis_block_hash.String())
		println("Aborting since cannot figure out the magic bytes")
		return
	}

	var total_data, curr_data int64

	for nd=tail; nd.Parent!=nil; {
		nd.Parent.Childs = []*chain.BlockTreeNode{nd}
		total_data += int64(nd.BlockSize)
		nd = nd.Parent
	}
	fmt.Println("Writting bootstrap.dat, height", tail.Height, "  magic", hex.EncodeToString(magic))
	f, _ := os.Create("bootstrap.dat")
	f.Write(magic)
	binary.Write(f, binary.LittleEndian, uint32(len(gen_bin)))
	f.Write(gen_bin)
	for {
		bl, _, _ := blks.BlockGet(nd.BlockHash)
		f.Write(magic)
		binary.Write(f, binary.LittleEndian, uint32(len(bl)))
		f.Write(bl)
		curr_data += int64(nd.BlockSize)
		if (nd.Height&0xfff)==0 {
			fmt.Printf("\r%.1f%%...", 100*float64(curr_data)/float64(total_data))
		}
		if len(nd.Childs)==0 {
			break
		}
		nd = nd.Childs[0]
	}
	fmt.Println("\rDone           ")
}
