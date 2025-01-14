package usif

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"math/rand"
	"sync"
	"time"

	"github.com/piotrnar/gocoin/client/common"
	"github.com/piotrnar/gocoin/client/network"
	"github.com/piotrnar/gocoin/client/network/peersdb"
	"github.com/piotrnar/gocoin/lib/btc"
	"github.com/piotrnar/gocoin/lib/others/qdb"
	"github.com/piotrnar/gocoin/lib/others/sys"
	"github.com/piotrnar/gocoin/lib/script"
)

type OneUiReq struct {
	Param   string
	Handler func(pars string)
	Done    sync.WaitGroup
}

// A thread that wants to lock the main thread calls:
// In.Add(1); Out.Add(1); [put msg into LocksChan]; In.Wait(); [do synchronized code]; Out.Done()
// The main thread, upon receiving the message, does:
// In.Done(); Out.Wait();
type OneLock struct {
	In  sync.WaitGroup // main thread calls Done() on this one and then Stop.Wait()
	Out sync.WaitGroup // the synchronized thread calls Done
}

var (
	UiChannel chan *OneUiReq = make(chan *OneUiReq, 1)
	LocksChan chan *OneLock  = make(chan *OneLock, 1)

	FetchingBalances sys.SyncBool
	Exit_now         sys.SyncBool

	print_buffer string
)

func hex_dump(d []byte) (s string) {
	for {
		le := 32
		if len(d) < le {
			le = len(d)
		}
		s += "       " + hex.EncodeToString(d[:le]) + "\n"
		d = d[le:]
		if len(d) == 0 {
			return
		}
	}
}

func dump_raw_sigscript(d []byte) bool {
	ss, er := btc.ScriptToText(d)
	if er != nil {
		println(er.Error())
		return false
	}

	p2sh := len(ss) >= 2 && d[0] == 0
	if p2sh {
		ms, er := btc.NewMultiSigFromScript(d)
		if er == nil {
			print_buffer += fmt.Sprintln("      Multisig script", ms.SigsNeeded, "of", len(ms.PublicKeys))
			for i := range ms.PublicKeys {
				print_buffer += fmt.Sprintf("       pkey%d = %s\n", i+1, hex.EncodeToString(ms.PublicKeys[i]))
			}
			for i := range ms.Signatures {
				print_buffer += fmt.Sprintf("       R%d = %64s\n", i+1, hex.EncodeToString(ms.Signatures[i].R.Bytes()))
				print_buffer += fmt.Sprintf("       S%d = %64s\n", i+1, hex.EncodeToString(ms.Signatures[i].S.Bytes()))
				print_buffer += fmt.Sprintf("       HashType%d = %02x\n", i+1, ms.Signatures[i].HashType)
			}
			return len(ms.Signatures) >= int(ms.SigsNeeded)
		} else {
			println(er.Error())
		}
	}

	print_buffer += fmt.Sprintln("      SigScript:")
	for i := range ss {
		if p2sh && i == len(ss)-1 {
			// Print p2sh script
			d, _ = hex.DecodeString(ss[i])
			s2, er := btc.ScriptToText(d)
			if er != nil {
				println(er.Error())
				p2sh = false
				print_buffer += fmt.Sprintln("       ", ss[i])
				continue
				//return
			}
			print_buffer += fmt.Sprintln("        P2SH spend script:")
			for j := range s2 {
				print_buffer += fmt.Sprintln("        ", s2[j])
			}
		} else {
			print_buffer += fmt.Sprintln("       ", ss[i])
		}
	}
	return true
}

func dump_sigscript(d []byte) bool {
	if len(d) == 0 {
		print_buffer += fmt.Sprintln("       WARNING: Empty sigScript")
		return false
	}
	rd := bytes.NewReader(d)

	// ECDSA Signature
	le, _ := rd.ReadByte()
	if le < 0x40 {
		return dump_raw_sigscript(d)
	}
	sd := make([]byte, le)
	_, er := rd.Read(sd)
	if er != nil {
		return dump_raw_sigscript(d)
	}
	sig, er := btc.NewSignature(sd)
	if er != nil {
		return dump_raw_sigscript(d)
	}
	print_buffer += fmt.Sprintf("       R = %64s\n", hex.EncodeToString(sig.R.Bytes()))
	print_buffer += fmt.Sprintf("       S = %64s\n", hex.EncodeToString(sig.S.Bytes()))
	print_buffer += fmt.Sprintf("       HashType = %02x\n", sig.HashType)

	// Key
	le, er = rd.ReadByte()
	if er != nil {
		print_buffer += fmt.Sprintln("       WARNING: PublicKey not present")
		print_buffer += fmt.Sprint(hex_dump(d))
		return false
	}

	sd = make([]byte, le)
	_, er = rd.Read(sd)
	if er != nil {
		print_buffer += fmt.Sprintln("       WARNING: PublicKey too short", er.Error())
		print_buffer += fmt.Sprint(hex_dump(d))
		return false
	}

	print_buffer += fmt.Sprintf("       PublicKeyType = %02x\n", sd[0])
	key, er := btc.NewPublicKey(sd)
	if er != nil {
		print_buffer += fmt.Sprintln("       WARNING: PublicKey broken", er.Error())
		print_buffer += fmt.Sprint(hex_dump(d))
		return false
	}
	print_buffer += fmt.Sprintf("       X = %64s\n", key.X.String())
	if le >= 65 {
		print_buffer += fmt.Sprintf("       Y = %64s\n", key.Y.String())
	}

	if rd.Len() != 0 {
		print_buffer += fmt.Sprintln("       WARNING: Extra bytes at the end of sigScript")
		print_buffer += fmt.Sprint(hex_dump(d[len(d)-rd.Len():]))
	}
	return true
}

func DecodeTxSops(tx *btc.Tx) (s string, missinginp bool, totinp, totout uint64, sigops uint, e error) {
	s += fmt.Sprintln("ID:", tx.Hash.String())
	s += fmt.Sprintln("WTxID:", tx.WTxID().String())
	s += fmt.Sprintln("Tx Version:", tx.Version)
	if tx.SegWit != nil {
		s += fmt.Sprintln("Segregated Witness transaction", len(tx.SegWit))
	} else {
		s += fmt.Sprintln("Regular (non-SegWit) transaction", len(tx.SegWit))
	}
	sigops = btc.WITNESS_SCALE_FACTOR * tx.GetLegacySigOpCount()
	tx.Spent_outputs = make([]*btc.TxOut, len(tx.TxIn))
	ss := make([]string, len(tx.TxIn))
	s += fmt.Sprintln("TX IN cnt:", len(tx.TxIn))
	for i := range tx.TxIn {
		ss[i] += fmt.Sprintf("%4d) %s\n     scr_len=%d    seq=%08x    ", i, tx.TxIn[i].Input.String(), len(tx.TxIn[i].ScriptSig), tx.TxIn[i].Sequence)
		var po *btc.TxOut

		inpid := btc.NewUint256(tx.TxIn[i].Input.Hash[:])
		if txinmem, ok := network.TransactionsToSend[inpid.BIdx()]; ok {
			ss[i] += "from mempool"
			if int(tx.TxIn[i].Input.Vout) >= len(txinmem.TxOut) {
				ss[i] += fmt.Sprintf(" - Vout TOO BIG (%d/%d)!", int(tx.TxIn[i].Input.Vout), len(txinmem.TxOut))
			} else {
				po = txinmem.TxOut[tx.TxIn[i].Input.Vout]
			}
		} else {
			po = common.BlockChain.Unspent.UnspentGet(&tx.TxIn[i].Input)
			if po != nil {
				ss[i] += fmt.Sprintf("from block %d", po.BlockHeight)
			}
		}
		tx.Spent_outputs[i] = po
	}
	var unsigned uint64
	for i := range tx.TxIn {
		s += ss[i]
		po := tx.Spent_outputs[i]
		if po != nil {
			ok := script.VerifyTxScript(po.Pk_script, &script.SigChecker{Amount: po.Value, Idx: i, Tx: tx}, script.VER_P2SH|script.VER_DERSIG|script.VER_CLTV)
			if !ok {
				s += fmt.Sprintln("\nERROR: The transacion does not have a valid signature.")
				e = errors.New("invalid signature")
			}
			totinp += po.Value

			ads := "???"
			if ad := btc.NewAddrFromPkScript(po.Pk_script, common.Testnet); ad != nil {
				ads = ad.String()
			}
			s += fmt.Sprintf("\n%15s BTC from address %s\n", btc.UintToBtc(po.Value), ads)

			if len(tx.TxIn[i].ScriptSig) > 0 {
				print_buffer = ""
				if !dump_sigscript(tx.TxIn[i].ScriptSig) {
					unsigned++
				}
				s += print_buffer
			} else {
				if tx.SegWit == nil || len(tx.SegWit[i]) < 2 {
					if i < len(tx.SegWit) && len(tx.SegWit[i]) == 1 && (len(tx.SegWit[i][0])|1) == 65 {
						s += fmt.Sprintln("      Schnorr signature:")
						s += fmt.Sprintln("       ", hex.EncodeToString(tx.SegWit[i][0][:32]))
						s += fmt.Sprintln("       ", hex.EncodeToString(tx.SegWit[i][0][32:]))
						if len(tx.SegWit[i][0]) == 65 {
							s += fmt.Sprintf("        Hash Type 0x%02x\n", tx.SegWit[i][0][64])
						}
						goto skip_wintesses
					} else {
						unsigned++
					}
				}
			}
			if tx.SegWit != nil {
				s += fmt.Sprintln("      Witness data:")
				for _, ww := range tx.SegWit[i] {
					if len(ww) == 0 {
						s += fmt.Sprintln("       ", "OP_0")
					} else {
						s += fmt.Sprintln("       ", hex.EncodeToString(ww))
					}
				}
			}
		skip_wintesses:

			if btc.IsP2SH(po.Pk_script) {
				so := btc.WITNESS_SCALE_FACTOR * btc.GetP2SHSigOpCount(tx.TxIn[i].ScriptSig)
				s += fmt.Sprintf("  + %d sigops", so)
				sigops += so
			}

			swo := tx.CountWitnessSigOps(i, po.Pk_script)
			if swo > 0 {
				s += fmt.Sprintf("  + %d segops", swo)
				sigops += swo
			}

			s += "\n"
		} else {
			s += fmt.Sprintln(" - UNKNOWN INPUT")
			missinginp = true
		}
	}
	s += fmt.Sprintln("TX OUT cnt:", len(tx.TxOut))
	for i := range tx.TxOut {
		totout += tx.TxOut[i].Value
		adr := btc.NewAddrFromPkScript(tx.TxOut[i].Pk_script, common.Testnet)
		if adr != nil {
			s += fmt.Sprintf("%4d) %20s BTC  to adr %s\n", i, btc.UintToBtc(tx.TxOut[i].Value),
				adr.String())
		} else {
			s += fmt.Sprintf("%4d) %20s BTC  to scr %s\n", i, btc.UintToBtc(tx.TxOut[i].Value),
				hex.EncodeToString(tx.TxOut[i].Pk_script))
		}
	}
	s += fmt.Sprintln("Lock Time:", tx.Lock_time)
	s += fmt.Sprintln("Transaction Size:", tx.Size, "   NoWitSize:", tx.NoWitSize,
		"   Weight:", tx.Weight(), "   VSize:", tx.VSize())

	if unsigned > 0 {
		s += fmt.Sprintln("WARNING:", unsigned, "out of", len(tx.TxIn), "inputs are not signed or signed only patially")
	} else {
		s += fmt.Sprintln("All", len(tx.TxIn), "transaction inputs seem to be signed")
	}

	if missinginp {
		s += fmt.Sprintln("WARNING: There are missing inputs and we cannot calc input BTC amount.")
		s += fmt.Sprintln("If there is somethign wrong with this transaction, you can loose money...")
	} else {
		total_fee := float64(totinp - totout)
		fee_spb := total_fee / float64(tx.VSize())
		s += fmt.Sprintf("All OK: %.8f BTC in -> %.8f BTC out, with %.8f BTC fee (%.2f SPB)\n",
			float64(totinp)/1e8, float64(totout)/1e8, total_fee/1e8, fee_spb)
		avg_fee := GetAverageFee()
		if fee_spb > 2*GetAverageFee() {
			s += fmt.Sprintf("WARNING: High fee SPB of %.02f (vs %.02f average).\n", fee_spb, avg_fee)
		}
	}

	s += fmt.Sprintln("ECDSA sig operations : ", sigops)

	return
}

func DecodeTx(tx *btc.Tx) (s string, missinginp bool, totinp, totout uint64, e error) {
	s, missinginp, totinp, totout, _, e = DecodeTxSops(tx)
	return
}

func LoadRawTx(buf []byte) (s string) {
	txd, er := hex.DecodeString(string(buf))
	if er != nil {
		txd = buf
	}

	// At this place we should have raw transaction in txd
	tx, le := btc.NewTx(txd)
	if tx == nil || le != len(txd) {
		s += fmt.Sprintln("Could not decode transaction file or it has some extra data")
		return
	}
	tx.SetHash(txd)

	s, _, _, _, _ = DecodeTx(tx)

	network.RemoveFromRejected(&tx.Hash) // in case we rejected it eariler, to try it again as trusted

	if why := network.NeedThisTxExt(&tx.Hash, nil); why != 0 {
		s += fmt.Sprintln("Transaction not needed or not wanted", why)
		network.TxMutex.Lock()
		if t2s := network.TransactionsToSend[tx.Hash.BIdx()]; t2s != nil {
			t2s.Local = true // make as own (if not needed)
		}
		network.TxMutex.Unlock()
		return
	}

	if !network.SubmitLocalTx(tx, txd) {
		network.TxMutex.Lock()
		rr := network.TransactionsRejected[tx.Hash.BIdx()]
		network.TxMutex.Unlock()
		if rr != nil {
			s += fmt.Sprintln("Transaction rejected", rr.Reason)
		} else {
			s += fmt.Sprintln("Transaction rejected in a weird way")
		}
		return
	}

	network.TxMutex.Lock()
	_, ok := network.TransactionsToSend[tx.Hash.BIdx()]
	network.TxMutex.Unlock()
	if ok {
		s += fmt.Sprintln("Transaction added to the memory pool. You can broadcast it now.")
	} else {
		s += fmt.Sprintln("Transaction not rejected, but also not accepted - very strange!")
	}

	return
}

func SendInvToRandomPeer(typ uint32, h *btc.Uint256) {
	common.CountSafe(fmt.Sprint("NetSendOneInv", typ))

	// Prepare the inv
	inv := new([36]byte)
	binary.LittleEndian.PutUint32(inv[0:4], typ)
	copy(inv[4:36], h.Bytes())

	// Append it to PendingInvs in a random connection
	network.Mutex_net.Lock()
	idx := rand.Intn(len(network.OpenCons))
	var cnt int
	for _, v := range network.OpenCons {
		if idx == cnt {
			v.Mutex.Lock()
			v.PendingInvs = append(v.PendingInvs, inv)
			v.Mutex.Unlock()
			break
		}
		cnt++
	}
	network.Mutex_net.Unlock()
}

func GetNetworkHashRateNum() float64 {
	hours := common.CFG.Stat.HashrateHrs
	common.Last.Mutex.Lock()
	end := common.Last.Block
	common.Last.Mutex.Unlock()
	now := time.Now().Unix()
	cnt := 0
	var diff float64
	for ; end != nil; cnt++ {
		if now-int64(end.Timestamp()) > int64(hours)*3600 {
			break
		}
		diff += btc.GetDifficulty(end.Bits())
		end = end.Parent
	}
	if cnt == 0 {
		return 0
	}
	diff /= float64(cnt)
	bph := float64(cnt) / float64(hours)
	return bph / 6 * diff * 7158278.826667
}

func ExecUiReq(req *OneUiReq) {
	if FetchingBalances.Get() {
		fmt.Println("Client is currently busy fetching wallet balance.\nYour command has been queued and will execute soon.")
	}

	//fmt.Println("main.go last seen in line", common.BusyIn())
	sta := time.Now().UnixNano()
	req.Done.Add(1)
	UiChannel <- req
	go func() {
		req.Done.Wait()
		sto := time.Now().UnixNano()
		fmt.Printf("Ready in %.3fs\n", float64(sto-sta)/1e9)
		fmt.Print("> ")
	}()
}

func MemoryPoolFees() (res string) {
	res = fmt.Sprintln("Content of mempool sorted by fee's SPB:")
	network.TxMutex.Lock()
	defer network.TxMutex.Unlock()

	sorted := network.GetSortedMempoolNew()

	var totlen, rawlen uint64
	for cnt := 0; cnt < len(sorted); cnt++ {
		v := sorted[cnt]
		newlen := totlen + uint64(v.VSize())
		rawlen += uint64(len(v.Raw))

		if cnt == 0 || cnt+1 == len(sorted) || (newlen/100e3) != (totlen/100e3) {
			spb := float64(v.Fee) / float64(v.VSize())
			toprint := newlen
			if cnt != 0 && cnt+1 != len(sorted) {
				toprint = newlen / 100e3 * 100e3
			}
			res += fmt.Sprintf(" %9d / %9d bytes, %6d txs @ fee %8.1f Satoshis / byte\n", toprint, rawlen, cnt+1, spb)
		}
		if (newlen / 1e6) != (totlen / 1e6) {
			res += "===========================================================\n"
		}

		totlen = newlen
	}
	return
}

// UnbanPeer unbans peer of a given IP or "all" banned peers
func UnbanPeer(par string) (s string) {
	var ad *peersdb.PeerAddr

	if par != "all" {
		var er error
		ad, er = peersdb.NewAddrFromString(par, false)
		if er != nil {
			s = fmt.Sprintln(par, er.Error())
			return
		}
		s += fmt.Sprintln("Unban", ad.Ip(), "...")
		network.HammeringMutex.Lock()
		delete(network.RecentlyDisconencted, ad.Ip4)
		network.HammeringMutex.Unlock()
	} else {
		s += fmt.Sprintln("Unban all peers ...")
		network.HammeringMutex.Lock()
		network.RecentlyDisconencted = make(map[[4]byte]*network.RecentlyDisconenctedType)
		network.HammeringMutex.Unlock()
	}

	var keys []qdb.KeyType
	var vals [][]byte
	peersdb.PeerDB.Browse(func(k qdb.KeyType, v []byte) uint32 {
		peer := peersdb.NewPeer(v)
		if peer.Banned != 0 {
			if ad == nil || peer.Ip() == ad.Ip() {
				s += fmt.Sprintln(" -", peer.NetAddr.String())
				peer.Banned = 0
				keys = append(keys, k)
				vals = append(vals, peer.Bytes())
			}
		}
		return 0
	})
	for i := range keys {
		peersdb.PeerDB.Put(keys[i], vals[i])
	}

	s += fmt.Sprintln(len(keys), "peer(s) un-baned")
	return
}

func GetReceivedBlockX(block *btc.Block) (rb *network.OneReceivedBlock, cbasetx *btc.Tx) {
	network.MutexRcv.Lock()
	rb = network.ReceivedBlocks[block.Hash.BIdx()]
	if rb.TheWeight == 0 {
		block.BuildTxListExt(false)
		rb.NonWitnessSize = block.NoWitnessSize
		rb.TheWeight = block.BlockWeight
		rb.ThePaidVSize = block.PaidTxsVSize
		rb.TheOrdCnt = block.OrbTxCnt
		rb.TheOrdSize = block.OrbTxSize
		rb.TheOrdWeight = block.OrbTxWeight
		cbasetx = block.Txs[0]
	} else {
		cbasetx, _ = btc.NewTx(block.Raw[block.TxOffset:])
	}
	network.MutexRcv.Unlock()
	return
}

func init() {
	rand.Seed(int64(time.Now().Nanosecond()))
}
