package utxo

import (
	"encoding/binary"
	"fmt"

	"github.com/piotrnar/gocoin/lib/btc"
)

const (
	UtxoIdxLen = 8 // Increase this value (maximum 32) for better security at a cost of memory usage
)

type UtxoKeyType [UtxoIdxLen]byte

type AllUnspentTx []*OneUnspentTx

// OneUnspentTx is returned by GetUnspentFromPkScr.
type OneUnspentTx struct {
	btc.TxPrevOut
	Value   uint64
	MinedAt uint32
	*btc.BtcAddr
	destString string
	Coinbase   bool
	Message    []byte
}

func (x AllUnspentTx) Len() int {
	return len(x)
}

func (x AllUnspentTx) Less(i, j int) bool {
	if x[i].MinedAt == x[j].MinedAt {
		if x[i].TxPrevOut.Hash == x[j].TxPrevOut.Hash {
			return x[i].TxPrevOut.Vout < x[j].TxPrevOut.Vout
		}
		return binary.LittleEndian.Uint64(x[i].TxPrevOut.Hash[24:32]) <
			binary.LittleEndian.Uint64(x[j].TxPrevOut.Hash[24:32])
	}
	return x[i].MinedAt < x[j].MinedAt
}

func (x AllUnspentTx) Swap(i, j int) {
	x[i], x[j] = x[j], x[i]
}

func (ou *OneUnspentTx) String() (s string) {
	s = fmt.Sprintf("%15s BTC %s", btc.UintToBtc(ou.Value), ou.TxPrevOut.String())
	if ou.BtcAddr != nil {
		s += " " + ou.DestAddr() + ou.BtcAddr.Label()
	}
	if ou.MinedAt != 0 {
		s += fmt.Sprint(" ", ou.MinedAt)
	}
	if ou.Coinbase {
		s += " Coinbase"
	}
	if ou.Message != nil {
		s += "  "
		for _, c := range ou.Message {
			if c < ' ' || c > 127 {
				s += fmt.Sprintf("\\x%02x", c)
			} else {
				s += string(c)
			}
		}
	}
	return
}

func (ou *OneUnspentTx) FixDestString() {
	ou.destString = ou.BtcAddr.String()
}

func (ou *OneUnspentTx) UnspentTextLine() (s string) {
	s = fmt.Sprintf("%s # %.8f BTC @ %s%s, block %d", ou.TxPrevOut.String(),
		float64(ou.Value)/1e8, ou.DestAddr(), ou.BtcAddr.Label(), ou.MinedAt)
	return
}

func (ou *OneUnspentTx) DestAddr() string {
	if ou.destString == "" {
		return ou.BtcAddr.String()
	}
	return ou.destString
}
