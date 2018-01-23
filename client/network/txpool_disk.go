package network

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"github.com/piotrnar/gocoin/client/common"
	"github.com/piotrnar/gocoin/lib/btc"
	"io"
	"os"
	"time"
)

var (
	END_MARKER = []byte("END_OF_FILE")
)

const (
	MEMPOOL_FILE_NAME2 = "mempool.dmp"
)

func bool2byte(v bool) byte {
	if v {
		return 1
	} else {
		return 0
	}
}

func (t2s *OneTxToSend) WriteBytes(wr io.Writer) {
	btc.WriteVlen(wr, uint64(len(t2s.Data)))
	wr.Write(t2s.Data)

	btc.WriteVlen(wr, uint64(len(t2s.Spent)))
	binary.Write(wr, binary.LittleEndian, t2s.Spent[:])

	binary.Write(wr, binary.LittleEndian, t2s.Invsentcnt)
	binary.Write(wr, binary.LittleEndian, t2s.SentCnt)
	binary.Write(wr, binary.LittleEndian, uint32(t2s.Firstseen.Unix()))
	binary.Write(wr, binary.LittleEndian, uint32(t2s.Lastsent.Unix()))
	binary.Write(wr, binary.LittleEndian, t2s.Volume)
	binary.Write(wr, binary.LittleEndian, t2s.Fee)
	binary.Write(wr, binary.LittleEndian, t2s.SigopsCost)
	binary.Write(wr, binary.LittleEndian, t2s.VerifyTime)
	wr.Write([]byte{t2s.Own, t2s.Blocked, bool2byte(t2s.MemInputs != nil), bool2byte(t2s.Final)})
}

func MempoolSave2() {
	if !common.CFG.TXPool.SaveOnDisk {
		os.Remove(common.GocoinHomeDir + MEMPOOL_FILE_NAME2)
		return
	}

	f, er := os.Create(common.GocoinHomeDir + MEMPOOL_FILE_NAME2)
	if er != nil {
		println(er.Error())
		return
	}

	fmt.Println("Saving", MEMPOOL_FILE_NAME2)
	wr := bufio.NewWriter(f)

	wr.Write(common.Last.Block.BlockHash.Hash[:])

	btc.WriteVlen(wr, uint64(len(TransactionsToSend)))
	for _, t2s := range TransactionsToSend {
		t2s.WriteBytes(wr)
	}

	btc.WriteVlen(wr, uint64(len(SpentOutputs)))
	for k, v := range SpentOutputs {
		binary.Write(wr, binary.LittleEndian, k)
		binary.Write(wr, binary.LittleEndian, v)
	}

	wr.Write(END_MARKER[:])
	wr.Flush()
	f.Close()
}

func MempoolLoad2() bool {
	var t2s *OneTxToSend
	var totcnt, le uint64
	var tmp [32]byte
	var bi BIDX
	var tina uint32
	var i int
	var cnt1, cnt2 uint

	f, er := os.Open(common.GocoinHomeDir + MEMPOOL_FILE_NAME2)
	if er != nil {
		fmt.Println("MempoolLoad:", er.Error())
		return false
	}
	defer f.Close()

	rd := bufio.NewReader(f)
	if er = btc.ReadAll(rd, tmp[:32]); er != nil {
		goto fatal_error
	}
	if !bytes.Equal(tmp[:32], common.Last.Block.BlockHash.Hash[:]) {
		er = errors.New(MEMPOOL_FILE_NAME2 + " is for different last block hash")
		goto fatal_error
	}

	if totcnt, er = btc.ReadVLen(rd); er != nil {
		goto fatal_error
	}

	TransactionsToSend = make(map[BIDX]*OneTxToSend, int(totcnt))
	for ; totcnt > 0; totcnt-- {
		le, er = btc.ReadVLen(rd)
		if er != nil {
			goto fatal_error
		}

		t2s = new(OneTxToSend)
		t2s.Data = make([]byte, int(le))

		er = btc.ReadAll(rd, t2s.Data)
		if er != nil {
			goto fatal_error
		}

		t2s.Tx, i = btc.NewTx(t2s.Data)
		if t2s.Tx == nil || i != len(t2s.Data) {
			er = errors.New(fmt.Sprint("Error parsing tx from ", MEMPOOL_FILE_NAME2, " at idx", len(TransactionsToSend)))
			goto fatal_error
		}
		t2s.Tx.SetHash(t2s.Data)

		le, er = btc.ReadVLen(rd)
		if er != nil {
			goto fatal_error
		}
		t2s.Spent = make([]uint64, int(le))
		if er = binary.Read(rd, binary.LittleEndian, t2s.Spent[:]); er != nil {
			goto fatal_error
		}

		if er = binary.Read(rd, binary.LittleEndian, &t2s.Invsentcnt); er != nil {
			goto fatal_error
		}

		if er = binary.Read(rd, binary.LittleEndian, &t2s.SentCnt); er != nil {
			goto fatal_error
		}

		if er = binary.Read(rd, binary.LittleEndian, &tina); er != nil {
			goto fatal_error
		}
		t2s.Firstseen = time.Unix(int64(tina), 0)

		if er = binary.Read(rd, binary.LittleEndian, &tina); er != nil {
			goto fatal_error
		}
		t2s.Lastsent = time.Unix(int64(tina), 0)

		if er = binary.Read(rd, binary.LittleEndian, &t2s.Volume); er != nil {
			goto fatal_error
		}

		if er = binary.Read(rd, binary.LittleEndian, &t2s.Fee); er != nil {
			goto fatal_error
		}

		if er = binary.Read(rd, binary.LittleEndian, &t2s.SigopsCost); er != nil {
			goto fatal_error
		}

		if er = binary.Read(rd, binary.LittleEndian, &t2s.VerifyTime); er != nil {
			goto fatal_error
		}

		if er = btc.ReadAll(rd, tmp[:4]); er != nil {
			goto fatal_error
		}
		t2s.Own = tmp[0]
		t2s.Blocked = tmp[1]
		if tmp[2] != 0 {
			t2s.MemInputs = make([]bool, len(t2s.TxIn))
		}
		t2s.Final = tmp[3] != 0

		t2s.Tx.Fee = t2s.Fee

		TransactionsToSend[t2s.Hash.BIdx()] = t2s
		TransactionsToSendSize += uint64(len(t2s.Data))
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

	if er = btc.ReadAll(rd, tmp[:len(END_MARKER)]); er != nil {
		goto fatal_error
	}
	if !bytes.Equal(tmp[:len(END_MARKER)], END_MARKER) {
		er = errors.New(MEMPOOL_FILE_NAME2 + " has marker missing")
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

	fmt.Println(len(TransactionsToSend), "transactions taking", TransactionsToSendSize, "Bytes loaded from", MEMPOOL_FILE_NAME2)
	fmt.Println(cnt1, "transactions use", cnt2, "memory inputs")

	return true

fatal_error:
	fmt.Println("Error loading", MEMPOOL_FILE_NAME2, ":", er.Error())
	TransactionsToSend = make(map[BIDX]*OneTxToSend)
	TransactionsToSendSize = 0
	TransactionsToSendWeight = 0
	SpentOutputs = make(map[uint64]BIDX)
	return false
}
