package network

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/piotrnar/gocoin/client/common"
	"github.com/piotrnar/gocoin/lib/btc"
)

var (
	END_MARKER = []byte("END_OF_FILE")
)

const (
	MEMPOOL_FILE_NAME = "mempool.dmp"
	FILE_VERSION_MAX  = 0xffffffff
	FILE_VERSION_3    = FILE_VERSION_MAX
	FILE_VERSION_4    = FILE_VERSION_3 - 1
	FILE_VERSION_CUR  = FILE_VERSION_4

	HAS_WAITING4_FLAG = 1 << 23
	HAS_TX_FLAG       = 1 << 22
)

func bool2byte(v bool) byte {
	if v {
		return 1
	} else {
		return 0
	}
}

func (t2s *OneTxToSend) WriteBytes(wr io.Writer) {
	btc.WriteVlen(wr, uint64(len(t2s.Raw)))
	wr.Write(t2s.Raw)

	btc.WriteVlen(wr, uint64(len(t2s.Spent)))
	binary.Write(wr, binary.LittleEndian, t2s.Spent[:])

	binary.Write(wr, binary.LittleEndian, t2s.Invsentcnt)
	binary.Write(wr, binary.LittleEndian, t2s.SentCnt)
	binary.Write(wr, binary.LittleEndian, uint32(t2s.Firstseen.Unix()))
	binary.Write(wr, binary.LittleEndian, uint32(t2s.Lastseen.Unix()))
	binary.Write(wr, binary.LittleEndian, uint32(t2s.Lastsent.Unix()))
	binary.Write(wr, binary.LittleEndian, t2s.Volume)
	binary.Write(wr, binary.LittleEndian, t2s.Fee)
	binary.Write(wr, binary.LittleEndian, t2s.SigopsCost)
	binary.Write(wr, binary.LittleEndian, t2s.VerifyTime)
	wr.Write([]byte{bool2byte(t2s.Local), t2s.Blocked, bool2byte(t2s.MemInputs != nil), bool2byte(t2s.Final)})
}

func (txr *OneTxRejected) WriteBytes(wr io.Writer) {
	wr.Write(txr.Id.Hash[:])
	binary.Write(wr, binary.LittleEndian, uint32(txr.Time.Unix()))

	// the next 32 bits is: (reason << 24) | has_waiting4 << 23 | has tx << 22
	tmp32 := txr.Size | (uint32(txr.Reason) << 24)
	if txr.Waiting4 != nil {
		tmp32 |= HAS_WAITING4_FLAG
	}
	if txr.Tx != nil {
		if txr.Size != uint32(len(txr.Tx.Raw)) {
			println("ERROR: Rejected Tx Size mismatch. THIS SHOUDL NOT HAPPEN - PLEASE REPORT!")
			println(txr.Id.String())
			println(txr.Tx.Hash.String())
			println(txr.Size, uint32(len(txr.Tx.Raw)))
			println(hex.EncodeToString(txr.Tx.Raw))
			txr.Tx = nil
		} else {
			tmp32 |= HAS_TX_FLAG
		}
	}
	binary.Write(wr, binary.LittleEndian, tmp32)
	if txr.Waiting4 != nil {
		wr.Write(txr.Waiting4.Hash[:])
	}
	if txr.Tx != nil {
		wr.Write(txr.Tx.Raw[:])
	}
}

func MempoolSave(force bool) {
	if !force && !common.CFG.TXPool.SaveOnDisk {
		os.Remove(common.GocoinHomeDir + MEMPOOL_FILE_NAME)
		return
	}

	f, er := os.Create(common.GocoinHomeDir + MEMPOOL_FILE_NAME)
	if er != nil {
		println(er.Error())
		return
	}

	fmt.Println("Saving", MEMPOOL_FILE_NAME)
	wr := bufio.NewWriter(f)

	wr.Write(common.Last.Block.BlockHash.Hash[:])

	btc.WriteVlen(wr, FILE_VERSION_CUR)

	btc.WriteVlen(wr, uint64(len(TransactionsToSend)))
	for _, t2s := range TransactionsToSend {
		t2s.WriteBytes(wr)
	}

	btc.WriteVlen(wr, uint64(len(SpentOutputs)))
	for k, v := range SpentOutputs {
		binary.Write(wr, binary.LittleEndian, k)
		binary.Write(wr, binary.LittleEndian, v)
	}

	btc.WriteVlen(wr, uint64(len(TransactionsRejected)))
	for _, v := range TransactionsRejected {
		v.WriteBytes(wr)
	}

	wr.Write(END_MARKER[:])
	wr.Flush()
	f.Close()
}

func newOneTxToSendFromFile(rd io.Reader, file_version int) (t2s *OneTxToSend, er error) {
	var tina uint32
	var i int
	var le uint64
	var tmp [4]byte

	t2s = new(OneTxToSend)

	if le, er = btc.ReadVLen(rd); er != nil {
		return
	}
	raw := make([]byte, int(le))

	if _, er = io.ReadFull(rd, raw); er != nil {
		return
	}

	t2s.Tx, i = btc.NewTx(raw)
	if t2s.Tx == nil || i != len(raw) {
		er = errors.New(fmt.Sprint("Error parsing tx from ", MEMPOOL_FILE_NAME, " at idx ", len(TransactionsToSend)))
		return
	}
	t2s.Tx.SetHash(raw)

	if le, er = btc.ReadVLen(rd); er != nil {
		return
	}
	t2s.Spent = make([]uint64, int(le))
	if er = binary.Read(rd, binary.LittleEndian, t2s.Spent[:]); er != nil {
		return
	}

	if er = binary.Read(rd, binary.LittleEndian, &t2s.Invsentcnt); er != nil {
		return
	}

	if er = binary.Read(rd, binary.LittleEndian, &t2s.SentCnt); er != nil {
		return
	}

	if er = binary.Read(rd, binary.LittleEndian, &tina); er != nil {
		return
	}
	t2s.Firstseen = time.Unix(int64(tina), 0)

	if file_version >= 3 {
		if er = binary.Read(rd, binary.LittleEndian, &tina); er != nil {
			return
		}
		t2s.Lastseen = time.Unix(int64(tina), 0)
	} else {
		t2s.Lastseen = time.Now()
	}

	if er = binary.Read(rd, binary.LittleEndian, &tina); er != nil {
		return
	}
	t2s.Lastsent = time.Unix(int64(tina), 0)

	if er = binary.Read(rd, binary.LittleEndian, &t2s.Volume); er != nil {
		return
	}

	if er = binary.Read(rd, binary.LittleEndian, &t2s.Fee); er != nil {
		return
	}

	if er = binary.Read(rd, binary.LittleEndian, &t2s.SigopsCost); er != nil {
		return
	}

	if er = binary.Read(rd, binary.LittleEndian, &t2s.VerifyTime); er != nil {
		return
	}

	if _, er = io.ReadFull(rd, tmp[:4]); er != nil {
		return
	}
	t2s.Local = tmp[0] != 0
	t2s.Blocked = tmp[1]
	if tmp[2] != 0 {
		t2s.MemInputs = make([]bool, len(t2s.TxIn))
	}
	t2s.Final = tmp[3] != 0

	t2s.Tx.Fee = t2s.Fee
	return
}

func newOneTxRejectedFromFile(rd io.Reader) (txr *OneTxRejected, er error) {
	var tina uint32
	var i int
	txr = new(OneTxRejected)
	txr.Id = new(btc.Uint256)
	if _, er = io.ReadFull(rd, txr.Id.Hash[:]); er != nil {
		return
	}
	if er = binary.Read(rd, binary.LittleEndian, &tina); er != nil {
		return
	}
	txr.Time = time.Unix(int64(tina), 0)
	if er = binary.Read(rd, binary.LittleEndian, &tina); er != nil {
		return
	}
	txr.Size = tina & (HAS_TX_FLAG - 1)
	txr.Reason = byte(tina >> 24)
	if (tina & HAS_WAITING4_FLAG) != 0 {
		txr.Waiting4 = new(btc.Uint256)
		if _, er = io.ReadFull(rd, txr.Waiting4.Hash[:]); er != nil {
			return
		}
	}
	if (tina & HAS_TX_FLAG) != 0 {
		raw := make([]byte, txr.Size)
		if _, er = io.ReadFull(rd, raw); er != nil {
			return
		}
		txr.Tx, i = btc.NewTx(raw)
		if txr.Tx == nil || i != len(raw) {
			er = errors.New(fmt.Sprint("Error parsing rejected tx from ", MEMPOOL_FILE_NAME, " at idx ", len(TransactionsRejected)))
			return
		}
		txr.Raw = raw
		txr.Tx.Hash.Hash = txr.Id.Hash
	}
	return
}

func MempoolLoad() bool {
	var t2s *OneTxToSend
	var txr *OneTxRejected
	var totcnt, le uint64
	var tmp [32]byte
	var bi BIDX
	var cnt1, cnt2 uint

	var file_version int

	f, er := os.Open(common.GocoinHomeDir + MEMPOOL_FILE_NAME)
	if er != nil {
		fmt.Println("MempoolLoad:", er.Error())
		return false
	}
	defer f.Close()

	rd := bufio.NewReader(f)
	if _, er = io.ReadFull(rd, tmp[:32]); er != nil {
		goto fatal_error
	}
	if !bytes.Equal(tmp[:32], common.Last.Block.BlockHash.Hash[:]) {
		er = errors.New(MEMPOOL_FILE_NAME + " is for different last block hash (try to load it with 'mpl' command)")
		goto fatal_error
	}

	if totcnt, er = btc.ReadVLen(rd); er != nil {
		goto fatal_error
	}

	if totcnt > FILE_VERSION_MAX/2 {
		if totcnt < FILE_VERSION_CUR {
			er = errors.New(MEMPOOL_FILE_NAME + " - new version of the file (not supported by this code)")
			goto fatal_error
		}
		file_version = FILE_VERSION_MAX - int(totcnt) + 3 // FILE_VERSION_MAX is vesion 3

		if totcnt, er = btc.ReadVLen(rd); er != nil {
			goto fatal_error
		}
	} else {
		file_version = 2 // version 2 did not have the version field
	}
	fmt.Println(MEMPOOL_FILE_NAME, "file version", file_version)

	TransactionsToSend = make(map[BIDX]*OneTxToSend, int(totcnt))
	for ; totcnt > 0; totcnt-- {
		if t2s, er = newOneTxToSendFromFile(rd, file_version); er != nil {
			goto fatal_error
		}
		TransactionsToSend[t2s.Hash.BIdx()] = t2s
		TransactionsToSendSize += uint64(len(t2s.Raw))
		TransactionsToSendWeight += uint64(t2s.Weight())
	}

	if totcnt, er = btc.ReadVLen(rd); er != nil {
		goto fatal_error
	}

	SpentOutputs = make(map[uint64]BIDX, int(totcnt))
	for ; totcnt > 0; totcnt-- {
		if er = binary.Read(rd, binary.LittleEndian, &le); er != nil {
			goto fatal_error
		}
		if er = binary.Read(rd, binary.LittleEndian, &bi); er != nil {
			goto fatal_error
		}
		SpentOutputs[le] = bi
	}

	if file_version >= 4 {
		if totcnt, er = btc.ReadVLen(rd); er != nil {
			goto fatal_error
		}
		TransactionsRejected = make(map[BIDX]*OneTxRejected, int(totcnt))
		for ; totcnt > 0; totcnt-- {
			if txr, er = newOneTxRejectedFromFile(rd); er != nil {
				goto fatal_error
			}
			TransactionsRejected[txr.Id.BIdx()] = txr
			TransactionsRejectedSize += uint64(txr.Size)
		}
	}

	if _, er = io.ReadFull(rd, tmp[:len(END_MARKER)]); er != nil {
		goto fatal_error
	}
	if !bytes.Equal(tmp[:len(END_MARKER)], END_MARKER) {
		er = errors.New(MEMPOOL_FILE_NAME + " has marker missing")
		goto fatal_error
	}

	// recover MemInputs
	for _, t2s := range TransactionsToSend {
		if t2s.MemInputs != nil {
			cnt1++
			for i := range t2s.TxIn {
				if _, inmem := TransactionsToSend[btc.BIdx(t2s.TxIn[i].Input.Hash[:])]; inmem {
					t2s.MemInputs[i] = true
					t2s.MemInputCnt++
					cnt2++
				}
			}
			if t2s.MemInputCnt == 0 {
				println("ERROR: MemInputs not nil but nothing found")
				t2s.MemInputs = nil
			}
		}
	}

	fmt.Println(len(TransactionsToSend), "transactions with total size of", TransactionsToSendSize, "bytes loaded from", MEMPOOL_FILE_NAME)
	//fmt.Println(cnt1, "transactions use", cnt2, "memory inputs")
	if len(TransactionsRejected) > 0 {
		fmt.Println("Also loaded", len(TransactionsRejected), "rejected transactions taking", TransactionsRejectedSize, "bytes")
	}

	return true

fatal_error:
	fmt.Println("Error loading", MEMPOOL_FILE_NAME, ":", er.Error())
	TransactionsToSend = make(map[BIDX]*OneTxToSend)
	TransactionsToSendSize = 0
	TransactionsToSendWeight = 0
	SpentOutputs = make(map[uint64]BIDX)
	return false
}

// MempoolLoadNew is only called from TextUI.
func MempoolLoadNew(fname string, abort *bool) bool {
	var ntx *TxRcvd
	var idx, totcnt, le, tmp64, oneperc, cntdwn, perc uint64
	var tmp [32]byte
	var tina uint32
	var i int
	var cnt1, cnt2 uint
	var t2s OneTxToSend

	f, er := os.Open(fname)
	if er != nil {
		fmt.Println("MempoolLoad:", er.Error())
		return false
	}
	defer f.Close()

	rd := bufio.NewReader(f)
	if _, er = io.ReadFull(rd, tmp[:32]); er != nil {
		goto fatal_error
	}

	if totcnt, er = btc.ReadVLen(rd); er != nil {
		goto fatal_error
	}
	fmt.Println("Loading", totcnt, "transactions from", fname)

	oneperc = totcnt / 100

	for idx = 0; idx < totcnt; idx++ {
		if cntdwn == 0 {
			fmt.Print("\r", perc, "% complete...")
			perc++
			cntdwn = oneperc
		}
		cntdwn--
		if abort != nil && *abort {
			break
		}
		le, er = btc.ReadVLen(rd)
		if er != nil {
			goto fatal_error
		}

		ntx = new(TxRcvd)
		raw := make([]byte, int(le))

		_, er = io.ReadFull(rd, raw)
		if er != nil {
			goto fatal_error
		}

		ntx.Tx, i = btc.NewTx(raw)
		if ntx.Tx == nil || i != len(raw) {
			er = errors.New(fmt.Sprint("Error parsing tx from ", fname, " at idx", idx))
			goto fatal_error
		}
		ntx.SetHash(raw)

		le, er = btc.ReadVLen(rd)
		if er != nil {
			goto fatal_error
		}

		for le > 0 {
			if er = binary.Read(rd, binary.LittleEndian, &tmp64); er != nil {
				goto fatal_error
			}
			le--
		}

		// discard all the rest...
		binary.Read(rd, binary.LittleEndian, &t2s.Invsentcnt)
		binary.Read(rd, binary.LittleEndian, &t2s.SentCnt)
		binary.Read(rd, binary.LittleEndian, &tina)
		binary.Read(rd, binary.LittleEndian, &tina)
		binary.Read(rd, binary.LittleEndian, &t2s.Volume)
		binary.Read(rd, binary.LittleEndian, &t2s.Fee)
		binary.Read(rd, binary.LittleEndian, &t2s.SigopsCost)
		binary.Read(rd, binary.LittleEndian, &t2s.VerifyTime)
		if _, er = io.ReadFull(rd, tmp[:4]); er != nil {
			goto fatal_error
		}

		// submit tx if we dont have it yet...
		if NeedThisTx(&ntx.Hash, nil) {
			cnt2++
			if HandleNetTx(ntx, true) {
				cnt1++
			}
		}
	}

	fmt.Print("\r                                    \r")
	fmt.Println(cnt1, "out of", cnt2, "new transactions accepted into memory pool")

	return true

fatal_error:
	fmt.Println("Error loading", fname, ":", er.Error())
	return false
}
