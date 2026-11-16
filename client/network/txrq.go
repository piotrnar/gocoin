package network

import (
	"encoding/binary"
	"slices"
	"sync"
	"time"

	"github.com/piotrnar/gocoin/client/common"
	"github.com/piotrnar/gocoin/client/txpool"
	"github.com/piotrnar/gocoin/lib/btc"
)

// When several peers announce the same transaction, we ask only the first one for
// it. The others are remembered and get asked (one by one) only if the tx does not
// arrive within TxRequestTimeout.

const (
	TxRequestTimeout    = 10 * time.Second // ask the next peer if the tx did not come within this time
	MaxTxRequests       = 20000            // above this many pending requests, we just ask everyone
	MaxTxRequestSources = 8                // how many other peers that have the tx we remember
)

type txRequest struct {
	asked  time.Time
	others []uint32 // ConnIDs of the other peers that announced this tx
	from   uint32   // ConnID of the peer we have asked
	hash   [32]byte
}

var (
	txRequestsMutex sync.Mutex
	txRequests      map[btc.BIDX]*txRequest = make(map[btc.BIDX]*txRequest)
)

// txRequestWanted is called when the peer announces a tx that we do not have.
// It returns true if we shall ask this peer for the tx now.
func (c *OneConnection) txRequestWanted(hash []byte) bool {
	idx := btc.NewUint256(hash).BIdx()
	txRequestsMutex.Lock()
	defer txRequestsMutex.Unlock()
	if r := txRequests[idx]; r != nil {
		if r.from == c.ConnID {
			return true // he announces it again, so just ask him again
		}
		if len(r.others) < MaxTxRequestSources && !slices.Contains(r.others, c.ConnID) {
			r.others = append(r.others, c.ConnID)
		}
		common.CountSafe("TxInvAskedOther")
		return false
	}
	if len(txRequests) < MaxTxRequests {
		r := &txRequest{asked: time.Now(), from: c.ConnID}
		copy(r.hash[:], hash)
		txRequests[idx] = r
	} else {
		common.CountSafe("TxRequestsFull")
	}
	return true
}

// txRequestDone must be called when a tx has been received from a peer.
func txRequestDone(idx btc.BIDX) {
	txRequestsMutex.Lock()
	delete(txRequests, idx)
	txRequestsMutex.Unlock()
}

// txRequestsTick asks the next peer for each tx that has not arrived on time.
func txRequestsTick(now time.Time) {
	txRequestsMutex.Lock()
	defer txRequestsMutex.Unlock()
	for idx, r := range txRequests {
		if now.Sub(r.asked) < TxRequestTimeout {
			continue
		}
		var asked bool
		if txpool.NeedThisTxExt(btc.NewUint256(r.hash[:]), nil) == 0 {
			for len(r.others) > 0 && !asked {
				c := GetConnFromID(r.others[0])
				r.others = r.others[1:]
				if c != nil && !c.IsBroken() {
					common.CountSafe("TxRequestRetry")
					c.sendGetTxs(r.hash[:])
					r.from = c.ConnID
					r.asked = now
					asked = true
				}
			}
		}
		if !asked {
			delete(txRequests, idx)
		}
	}
}

// sendGetTxs sends "getdata" for the given transactions (txids, 32 bytes each).
func (c *OneConnection) sendGetTxs(hashes []byte) {
	cnt := len(hashes) / 32
	if cnt == 0 {
		return
	}
	msg := make([]byte, 9, 9+36*cnt)
	msg = msg[:btc.PutVlen(msg, cnt)]
	for i := 0; i < cnt; i++ {
		msg = binary.LittleEndian.AppendUint32(msg, MSG_WITNESS_TX) // SegWit Tx
		msg = append(msg, hashes[32*i:32*i+32]...)
	}
	c.SendRawMsg("getdata", msg, false)
}
