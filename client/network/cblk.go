package network

import (
	"fmt"
	"time"
	"sync"
	"bytes"
	"io/ioutil"
	"encoding/hex"
	"crypto/sha256"
	"encoding/binary"
	"github.com/dchest/siphash"
	"github.com/piotrnar/gocoin/lib/btc"
	"github.com/piotrnar/gocoin/lib/chain"
	"github.com/piotrnar/gocoin/client/common"
)

var (
	CompactBlocksMutex sync.Mutex
)


type CmpctBlockCollector struct {
	Header []byte
	Txs []interface{} // either []byte of uint64
	K0, K1 uint64
	Sid2idx map[uint64]int
	Missing int
}

func ShortIDToU64(d []byte) uint64 {
	return uint64(d[0]) | (uint64(d[1])<<8) | (uint64(d[2])<<16) |
		(uint64(d[3])<<24) | (uint64(d[4])<<32) | (uint64(d[5])<<40)
}

func (col *CmpctBlockCollector) Assemble() []byte {
	bdat := new(bytes.Buffer)
	bdat.Write(col.Header)
	btc.WriteVlen(bdat, uint64(len(col.Txs)))
	for _, txd := range col.Txs {
		bdat.Write(txd.([]byte))
	}
	return bdat.Bytes()
}

func GetchBlockForBIP152(hash *btc.Uint256) (crec *chain.BlckCachRec) {
	var er error

	CompactBlocksMutex.Lock()
	defer CompactBlocksMutex.Unlock()

	crec, _, er = common.BlockChain.Blocks.BlockGetExt(hash)
	if crec==nil{
		fmt.Println("BlockGetExt failed for", hash.String(), er.Error())
		return
	}

	if crec.Block==nil {
		crec.Block, _ = btc.NewBlock(crec.Data)
		if crec.Block==nil {
			fmt.Println("GetchBlockForBIP152: btc.NewBlock() failed for", hash.String())
			return
		}
	}

	if len(crec.Block.Txs)==0 {
		if crec.Block.BuildTxList()!=nil {
			fmt.Println("GetchBlockForBIP152: bl.BuildTxList() failed for", hash.String())
			return
		}
	}

	if len(crec.BIP152)!=24 {
		crec.BIP152 = make([]byte, 24)
		copy(crec.BIP152[:8], crec.Data[48:56]) // set the nonce to 8 middle-bytes of block's merkle_root
		sha := sha256.New()
		sha.Write(crec.Data[:80])
		sha.Write(crec.BIP152[:8])
		copy(crec.BIP152[8:24], sha.Sum(nil)[0:16])
	}

	return
}

func (c *OneConnection) SendCmpctBlk(hash *btc.Uint256) bool {
	crec := GetchBlockForBIP152(hash)
	if crec==nil {
		//fmt.Println(c.ConnID, "cmpctblock not sent:", c.Node.Agent, hash.String())
		return false
	}

	k0 := binary.LittleEndian.Uint64(crec.BIP152[8:16])
	k1 := binary.LittleEndian.Uint64(crec.BIP152[16:24])

	msg := new(bytes.Buffer)
	msg.Write(crec.Data[:80])
	msg.Write(crec.BIP152[:8])
	btc.WriteVlen(msg, uint64(len(crec.Block.Txs)-1)) // all except coinbase
	for i:=1; i<len(crec.Block.Txs); i++ {
		var lsb [8]byte
		var hasz *btc.Uint256
		if c.Node.SendCmpctVer==2 {
			hasz = crec.Block.Txs[i].WTxID()
		} else {
			hasz = crec.Block.Txs[i].Hash
		}
		binary.LittleEndian.PutUint64(lsb[:], siphash.Hash(k0, k1, hasz.Hash[:]))
		msg.Write(lsb[:6])
	}
	msg.Write([]byte{1}) // one preffiled tx
	msg.Write([]byte{0}) // coinbase - index 0
	if c.Node.SendCmpctVer==2 {
		msg.Write(crec.Block.Txs[0].Raw) // coinbase - index 0
	} else {
		crec.Block.Txs[0].WriteSerialized(msg) // coinbase - index 0
	}
	c.SendRawMsg("cmpctblock", msg.Bytes())
	return true
}

func (c *OneConnection) ProcessGetBlockTxn(pl []byte) {
	if len(pl)<34 {
		println(c.ConnID, "GetBlockTxnShort")
		c.DoS("GetBlockTxnShort")
		return
	}
	hash := btc.NewUint256(pl[:32])
	crec := GetchBlockForBIP152(hash)
	if crec==nil {
		fmt.Println(c.ConnID, "GetBlockTxn aborting for", hash.String())
		return
	}

	req := bytes.NewReader(pl[32:])
	indexes_length, _ := btc.ReadVLen(req)
	if indexes_length==0 {
		println(c.ConnID, "GetBlockTxnEmpty")
		c.DoS("GetBlockTxnEmpty")
		return
	}

	var exp_idx uint64
	msg := new(bytes.Buffer)

	msg.Write(hash.Hash[:])
	btc.WriteVlen(msg, indexes_length)

	for {
		idx, er := btc.ReadVLen(req)
		if er != nil {
			println(c.ConnID, "GetBlockTxnERR")
			c.DoS("GetBlockTxnERR")
			return
		}
		idx += exp_idx
		if int(idx) >= len(crec.Block.Txs) {
			println(c.ConnID, "GetBlockTxnIdx+")
			c.DoS("GetBlockTxnIdx+")
			return
		}
		if c.Node.SendCmpctVer==2 {
			msg.Write(crec.Block.Txs[idx].Raw) // coinbase - index 0
		} else {
			crec.Block.Txs[idx].WriteSerialized(msg) // coinbase - index 0
		}
		if indexes_length==1 {
			break
		}
		indexes_length--
		exp_idx = idx+1
	}

	c.SendRawMsg("blocktxn", msg.Bytes())
}

func delB2G_callback(hash *btc.Uint256) {
	DelB2G(hash.BIdx())
}

func (c *OneConnection) ProcessCmpctBlock(pl []byte) {
	if len(pl)<90 {
		println(c.ConnID, c.PeerAddr.Ip(), c.Node.Agent, "cmpctblock error A", hex.EncodeToString(pl))
		c.DoS("CmpctBlkErrA")
		return
	}

	MutexRcv.Lock()
	defer MutexRcv.Unlock()


	var tmp_hdr [81]byte
	copy(tmp_hdr[:80], pl[:80])
	sta, b2g := c.ProcessNewHeader(tmp_hdr[:]) // ProcessNewHeader() needs byte(0) after the header,
	// but don't try to change it to ProcessNewHeader(append(pl[:80], 0)) as it'd overwrite pl[80]

	if b2g==nil {
		/*fmt.Println(c.ConnID, "Don't process CompactBlk", btc.NewSha2Hash(pl[:80]),
			hex.EncodeToString(pl[80:88]), "->", sta)*/
		common.CountSafe("CmpctBlockHdrNo")
		if sta==PH_STATUS_ERROR {
			c.ReceiveHeadersNow() // block doesn't connect so ask for the headers
			c.Misbehave("BadCmpct", 50) // do it 20 times and you are banned
		} else if sta==PH_STATUS_FATAL {
			c.DoS("BadCmpct")
		}
		return
	}
	if sta==PH_STATUS_NEW {
		b2g.SendInvs = true
	}

	/*fmt.Println()
	fmt.Println(c.ConnID, "Received CmpctBlock  enf:", common.BlockChain.Consensus.Enforce_SEGWIT,
		"   serwice:", (c.Node.Services&SERVICE_SEGWIT)!=0,
		"   height:", b2g.Block.Height,
		"   ver:", c.Node.SendCmpctVer)
	*/
	if common.BlockChain.Consensus.Enforce_SEGWIT!=0 && c.Node.SendCmpctVer < 2 {
		if b2g.Block.Height >= common.BlockChain.Consensus.Enforce_SEGWIT {
			common.CountSafe("CmpctBlockIgnore")
			println("Ignore compact block", b2g.Block.Height, "from non-segwit node", c.ConnID)
			if (c.Node.Services&SERVICE_SEGWIT)!=0 {
				// it only makes sense to ask this node for block's data, if it supports segwit
				c.MutexSetBool(&c.X.GetBlocksDataNow, true)
			}
			return
		}
	}

	/*
	var sta_s = []string{"???", "NEW", "FRESH", "OLD", "ERROR", "FATAL"}
	fmt.Println(c.ConnID, "Compact Block len", len(pl), "for", btc.NewSha2Hash(pl[:80]).String()[48:],
		"#", b2g.Block.Height, sta_s[sta], " inp:", b2g.InProgress)
	*/

	// if we got here, we shall download this block
	if c.Node.Height < b2g.Block.Height {
		c.Mutex.Lock()
		c.Node.Height = b2g.Block.Height
		c.Mutex.Unlock()
	}

	if b2g.InProgress >= uint(common.CFG.Net.MaxBlockAtOnce) {
		common.CountSafe("CmpctBlockMaxInProg")
		//fmt.Println(c.ConnID, " - too many in progress")
		return
	}

	var n, idx, shortidscnt, shortidx_idx, prefilledcnt int

	col := new(CmpctBlockCollector)
	col.Header = b2g.Block.Raw[:80]

	offs := 88
	shortidscnt, n = btc.VLen(pl[offs:])
	if shortidscnt<0 || n>3 {
		println(c.ConnID, c.PeerAddr.Ip(), c.Node.Agent, "cmpctblock error B", hex.EncodeToString(pl))
		c.DoS("CmpctBlkErrB")
		return
	}
	offs += n
	shortidx_idx = offs
	shortids := make(map[uint64] *OneTxToSend, shortidscnt)
	for i:=0; i<int(shortidscnt); i++ {
		if len(pl[offs:offs+6])<6 {
			println(c.ConnID, c.PeerAddr.Ip(), c.Node.Agent, "cmpctblock error B2", hex.EncodeToString(pl))
			c.DoS("CmpctBlkErrB2")
			return
		}
		shortids[ShortIDToU64(pl[offs:offs+6])] = nil
		offs += 6
	}

	prefilledcnt, n = btc.VLen(pl[offs:])
	if prefilledcnt<0 || n>3 {
		println(c.ConnID, c.PeerAddr.Ip(), c.Node.Agent, "cmpctblock error C", hex.EncodeToString(pl))
		c.DoS("CmpctBlkErrC")
		return
	}
	offs += n

	col.Txs = make([]interface{}, prefilledcnt+shortidscnt)

	exp := 0
	for i:=0; i<int(prefilledcnt); i++ {
		idx, n = btc.VLen(pl[offs:])
		if idx<0 || n>3 {
			println(c.ConnID, c.PeerAddr.Ip(), c.Node.Agent, "cmpctblock error D", hex.EncodeToString(pl))
			c.DoS("CmpctBlkErrD")
			return
		}
		idx += exp
		offs += n
		n = btc.TxSize(pl[offs:])
		if n==0 {
			println(c.ConnID, c.PeerAddr.Ip(), c.Node.Agent, "cmpctblock error E", hex.EncodeToString(pl))
			c.DoS("CmpctBlkErrE")
			return
		}
		col.Txs[idx] = pl[offs:offs+n]
		//fmt.Println("  prefilledtxn", i, idx, ":", hex.EncodeToString(pl[offs:offs+n]))
		//btc.NewSha2Hash(pl[offs:offs+n]).String())
		offs += n
		exp = int(idx)+1
	}

	// calculate K0 and K1 params for siphash-4-2
	sha := sha256.New()
	sha.Write(pl[:88])
	kks := sha.Sum(nil)
	col.K0 = binary.LittleEndian.Uint64(kks[0:8])
	col.K1 = binary.LittleEndian.Uint64(kks[8:16])

	var cnt_found int

	TxMutex.Lock()

	for _, v := range TransactionsToSend {
		var hash2take *btc.Uint256
		if c.Node.SendCmpctVer==2 {
			hash2take = v.Tx.WTxID()
		} else {
			hash2take = v.Tx.Hash
		}
		sid := siphash.Hash(col.K0, col.K1, hash2take.Hash[:]) & 0xffffffffffff
		if ptr, ok := shortids[sid]; ok {
			if ptr!=nil {
				common.CountSafe("ShortIDSame")
				println(c.ConnID, c.PeerAddr.Ip(), c.Node.Agent, "Same short ID - abort")
				return
			}
			shortids[sid] = v
			cnt_found++
		}
	}

	var msg *bytes.Buffer

	missing := len(shortids) - cnt_found
	//fmt.Println(c.ConnID, c.Node.SendCmpctVer, "ShortIDs", cnt_found, "/", shortidscnt, "  Prefilled", prefilledcnt, "  Missing", missing, "  MemPool:", len(TransactionsToSend))
	col.Missing = missing
	if missing > 0 {
		msg = new(bytes.Buffer)
		msg.Write(b2g.Block.Hash.Hash[:])
		btc.WriteVlen(msg, uint64(missing))
		exp = 0
		col.Sid2idx = make(map[uint64]int, missing)
	}
	for n=0; n<len(col.Txs); n++ {
		switch col.Txs[n].(type) {
			case []byte: // prefilled transaction

			default:
				sid := ShortIDToU64(pl[shortidx_idx:shortidx_idx+6])
				if t2s, ok := shortids[sid]; ok {
					if t2s!=nil {
						col.Txs[n] = t2s.Data
					} else {
						col.Txs[n] = sid
						col.Sid2idx[sid] = n
						if missing > 0 {
							btc.WriteVlen(msg, uint64(n-exp))
							exp = n+1
						}
					}
				} else {
					panic(fmt.Sprint("Tx idx ", n, " is missing - this should not happen!!!"))
				}
				shortidx_idx += 6
		}
	}
	TxMutex.Unlock()

	if missing==0 {
		//sta := time.Now()
		b2g.Block.UpdateContent(col.Assemble())
		//sto := time.Now()
		bidx := b2g.Block.Hash.BIdx()
		er := common.BlockChain.PostCheckBlock(b2g.Block)
		if er!=nil {
			println(c.ConnID, "Corrupt CmpctBlkA")
			ioutil.WriteFile(b2g.Hash.String()+".bin", b2g.Block.Raw, 0700)

			if b2g.Block.MerkleRootMatch() {
				println("It was a wrongly mined one - clean it up")
				DelB2G(bidx) //remove it from BlocksToGet
				if b2g.BlockTreeNode==LastCommitedHeader {
					LastCommitedHeader = LastCommitedHeader.Parent
				}
				common.BlockChain.DeleteBranch(b2g.BlockTreeNode, delB2G_callback)
			}

			//c.DoS("BadCmpctBlockA")
			return
		}
		//fmt.Println(c.ConnID, "Instatnt PostCheckBlock OK #", b2g.Block.Height, sto.Sub(sta), time.Now().Sub(sta))
		c.Mutex.Lock()
		c.counters["NewCBlock"]++
		c.blocksreceived = append(c.blocksreceived, time.Now())
		c.Mutex.Unlock()
		orb := &OneReceivedBlock{TmStart:b2g.Started, TmPreproc:time.Now(), FromConID:c.ConnID, DoInvs:b2g.SendInvs}
		ReceivedBlocks[bidx] = orb
		DelB2G(bidx) //remove it from BlocksToGet if no more pending downloads
		NetBlocks <- &BlockRcvd{Conn:c, Block:b2g.Block, BlockTreeNode:b2g.BlockTreeNode, OneReceivedBlock:orb}
	} else {
		if b2g.TmPreproc.IsZero() { // do not overwrite TmPreproc if already set
			b2g.TmPreproc = time.Now()
		}
		b2g.InProgress++
		c.Mutex.Lock()
		c.GetBlockInProgress[b2g.Block.Hash.BIdx()] = &oneBlockDl{hash:b2g.Block.Hash, start:time.Now(), col:col}
		c.Mutex.Unlock()
		c.SendRawMsg("getblocktxn", msg.Bytes())
		//fmt.Println(c.ConnID, "Send getblocktxn for", col.Missing, "/", shortidscnt, "missing txs.  ", msg.Len(), "bytes")
	}
}


func (c *OneConnection) ProcessBlockTxn(pl []byte) {
	if len(pl)<33 {
		println(c.ConnID, c.PeerAddr.Ip(), c.Node.Agent, "blocktxn error A", hex.EncodeToString(pl))
		c.DoS("BlkTxnErrLen")
		return
	}
	hash := btc.NewUint256(pl[:32])
	le, n := btc.VLen(pl[32:])
	if le<0 || n>3 {
		println(c.ConnID, c.PeerAddr.Ip(), c.Node.Agent, "blocktxn error B", hex.EncodeToString(pl))
		c.DoS("BlkTxnErrCnt")
		return
	}
	MutexRcv.Lock()
	defer MutexRcv.Unlock()

	idx := hash.BIdx()

	c.Mutex.Lock()
	bip := c.GetBlockInProgress[idx]
	if bip==nil {
		c.Mutex.Unlock()
		println(c.ConnID, "blocktxn received from", c.PeerAddr.Ip(), "while block not in progress")
		c.counters["BlkTxnNoBIP"]++
		c.Misbehave("BlkTxnErrBip", 100)
		return
	}
	col := bip.col
	if col==nil {
		c.Mutex.Unlock()
		println("blocktxn received from", c.PeerAddr.Ip(), "while we have no col object for it")
		common.CountSafe("UnxpectedBlockTxn")
		c.counters["BlkTxnNoCOL"]++
		c.Misbehave("BlkTxnNoCOL", 100)
		return
	}
	delete(c.GetBlockInProgress, idx)
	c.Mutex.Unlock()

	// the blocks seems to be fine
	if rb, got := ReceivedBlocks[idx]; got {
		rb.Cnt++
		common.CountSafe("BlkTxnSameRcvd")
		//fmt.Println(c.ConnID, "BlkTxn size", len(pl), "for", hash.String()[48:],"- already have")
		return
	}

	b2g := BlocksToGet[idx]
	if b2g==nil {
		panic("BlockTxn: Block missing from BlocksToGet")
		return
	}
	//b2g.InProgress--

	//fmt.Println(c.ConnID, "BlockTxn size", len(pl), "-", le, "new txs for block #", b2g.Block.Height)

	offs := 32+n
	for offs < len(pl) {
		n = btc.TxSize(pl[offs:])
		if n==0 {
			println(c.ConnID, c.PeerAddr.Ip(), c.Node.Agent, "blocktxn corrupt TX")
			c.DoS("BlkTxnErrTx")
			return
		}
		raw_tx := pl[offs:offs+n]
		tx_hash := btc.NewSha2Hash(raw_tx)
		offs += n

		sid := siphash.Hash(col.K0, col.K1, tx_hash.Hash[:]) & 0xffffffffffff
		if idx, ok := col.Sid2idx[sid]; ok {
			col.Txs[idx] = raw_tx
		} else {
			common.CountSafe("ShortIDUnknown")
			println(c.ConnID, c.PeerAddr.Ip(), c.Node.Agent, "blocktxn TX (short) ID unknown")
			return
		}
	}

	//println(c.ConnID, "Received the rest of compact block version", c.Node.SendCmpctVer)

	//sta := time.Now()
	b2g.Block.UpdateContent(col.Assemble())
	//sto := time.Now()
	er := common.BlockChain.PostCheckBlock(b2g.Block)
	if er!=nil {
		println(c.ConnID, c.PeerAddr.Ip(), c.Node.Agent, "Corrupt CmpctBlkB")
		//c.DoS("BadCmpctBlockB")
		ioutil.WriteFile(b2g.Hash.String()+".bin", b2g.Block.Raw, 0700)

		if b2g.Block.MerkleRootMatch() {
			println("It was a wrongly mined one - clean it up")
			DelB2G(idx) //remove it from BlocksToGet
			if b2g.BlockTreeNode==LastCommitedHeader {
				LastCommitedHeader = LastCommitedHeader.Parent
			}
			common.BlockChain.DeleteBranch(b2g.BlockTreeNode, delB2G_callback)
		}

		return
	}
	DelB2G(idx)
	//fmt.Println(c.ConnID, "PostCheckBlock OK #", b2g.Block.Height, sto.Sub(sta), time.Now().Sub(sta))
	c.Mutex.Lock()
	c.counters["NewTBlock"]++
	c.blocksreceived = append(c.blocksreceived, time.Now())
	c.Mutex.Unlock()
	orb := &OneReceivedBlock{TmStart:b2g.Started, TmPreproc:b2g.TmPreproc,
		TmDownload:c.LastMsgTime, TxMissing:col.Missing, FromConID:c.ConnID, DoInvs:b2g.SendInvs}
	ReceivedBlocks[idx] = orb
	NetBlocks <- &BlockRcvd{Conn:c, Block:b2g.Block, BlockTreeNode:b2g.BlockTreeNode, OneReceivedBlock:orb}
}
