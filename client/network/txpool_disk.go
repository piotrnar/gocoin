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
	FILE_VERSION_MAX  = 0xffffffff // this is version 3
	FILE_VERSION_9    = FILE_VERSION_MAX - 6
	FILE_VERSION_CUR  = FILE_VERSION_9

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
	TxMutex.Lock() // this should not be needed in our application, but just to have everything consistant
	defer TxMutex.Unlock()

	if !force && !common.CFG.TXPool.SaveOnDisk {
		os.Remove(common.GocoinHomeDir + MEMPOOL_FILE_NAME)
		return
	}

	f, er := os.Create(common.GocoinHomeDir + MEMPOOL_FILE_NAME)
	if er != nil {
		println(er.Error())
		return
	}

	fmt.Println("Saving", MEMPOOL_FILE_NAME, "version", FILE_VERSION_MAX-FILE_VERSION_CUR+3)
	wr := bufio.NewWriter(f)

	wr.Write(common.Last.Block.BlockHash.Hash[:])

	btc.WriteVlen(wr, FILE_VERSION_CUR)

	btc.WriteVlen(wr, uint64(len(TransactionsToSend)))
	for _, t2s := range TransactionsToSend {
		t2s.WriteBytes(wr)
	}

	btc.WriteVlen(wr, uint64(len(TransactionsRejected)))
	for idx := TRIdxTail; ; idx = TRIdxNext(idx) {
		if txr := TransactionsRejected[TRIdxArray[idx]]; txr != nil {
			txr.WriteBytes(wr)
		}
		if idx == TRIdxHead {
			break
		}
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

	if file_version < 6 {
		if le, er = btc.ReadVLen(rd); er != nil {
			return
		}
		spent := make([]uint64, int(le))
		if er = binary.Read(rd, binary.LittleEndian, spent); er != nil {
			return
		}
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

	//fmt.Println(" ", len(TransactionsRejected), "+txr", txr.Id.String(), txr.Size, ReasonToString(txr.Reason), (tina&HAS_WAITING4_FLAG) != 0, (tina&HAS_TX_FLAG) != 0)
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
	} else if txr.Waiting4 != nil {
		println("WARNING: RejectedTx", txr.Id.String(), "was waiting for inputs, but has no data")
		txr.Waiting4 = nil
	}
	txr.Footprint = uint32(txr.SysSize())

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

	InitMempool()

	TxMutex.Lock() // this should not be needed in our application, but just to have everything consistant
	defer TxMutex.Unlock()

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
		file_version = FILE_VERSION_MAX - int(totcnt) + 3 // FILE_VERSION_MAX is vesion 3

		if totcnt, er = btc.ReadVLen(rd); er != nil {
			goto fatal_error
		}
	} else {
		file_version = 2 // version 2 did not have the version field
	}
	fmt.Println(MEMPOOL_FILE_NAME, "file version", file_version)
	if file_version != 9 {
		er = errors.New("file version not supported")
		goto fatal_error
	}

	//println("TransactionsToSend cnt:", totcnt)
	TransactionsToSend = make(map[BIDX]*OneTxToSend, int(totcnt))
	for ; totcnt > 0; totcnt-- {
		if t2s, er = newOneTxToSendFromFile(rd, file_version); er != nil {
			goto fatal_error
		}
		t2s.Footprint = uint32(t2s.SysSize())
		TransactionsToSend[t2s.Hash.BIdx()] = t2s
		TransactionsToSendSize += uint64(t2s.Footprint)
		TransactionsToSendWeight += uint64(t2s.Weight())
	}

	if file_version < 5 {
		if totcnt, er = btc.ReadVLen(rd); er != nil {
			goto fatal_error
		}
		println("SpentOutputs cnt:", totcnt, "- discarding")
		for ; totcnt > 0; totcnt-- {
			if er = binary.Read(rd, binary.LittleEndian, &le); er != nil {
				goto fatal_error
			}
			if er = binary.Read(rd, binary.LittleEndian, &bi); er != nil {
				goto fatal_error
			}
		}
	}

	//fmt.Println("Rebuilding SpentOutputs")
	SpentOutputs = make(map[uint64]BIDX, 4*len(TransactionsToSend))
	for bidx, t2s := range TransactionsToSend {
		for _, inp := range t2s.TxIn {
			SpentOutputs[inp.Input.UIdx()] = bidx
		}
	}

	if file_version >= 4 {
		if totcnt, er = btc.ReadVLen(rd); er != nil {
			goto fatal_error
		}
		for ; totcnt > 0; totcnt-- {
			if txr, er = newOneTxRejectedFromFile(rd); er != nil {
				goto fatal_error
			}
			AddRejectedTx(txr)
		}
	}

	if _, er = io.ReadFull(rd, tmp[:len(END_MARKER)]); er != nil {
		goto fatal_error
	}

	if !bytes.Equal(tmp[:len(END_MARKER)], END_MARKER) {
		er = errors.New(MEMPOOL_FILE_NAME + " has marker missing")
		println("marker error", string(tmp[:len(END_MARKER)]))
		println(hex.EncodeToString(tmp[:len(END_MARKER)]))
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
				t2s.Footprint = uint32(t2s.SysSize())
			}
		}
	}

	fmt.Println(len(TransactionsToSend), "transactions with total size of", TransactionsToSendSize, "bytes loaded")
	//fmt.Println(cnt1, "transactions use", cnt2, "memory inputs")
	if len(TransactionsRejected) > 0 {
		fmt.Println("Additionally loaded", len(TransactionsRejected), "rejected transactions taking", TransactionsRejectedSize, "bytes")
	}

	//println("***Remove this: Mempool error after loading:", MempoolCheck())
	return true

fatal_error:
	fmt.Println("Error loading", MEMPOOL_FILE_NAME, ":", er.Error())
	TransactionsToSend = make(map[BIDX]*OneTxToSend)
	TransactionsToSendSize = 0
	TransactionsToSendWeight = 0
	SpentOutputs = make(map[uint64]BIDX)
	return false
}
