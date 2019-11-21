package btc

import (
	"encoding/binary"
	"fmt"
)

type AllUnspentTx []*OneUnspentTx

// OneUnspentTx is returned by GetUnspentFromPkScr.
type OneUnspentTx struct {
	TxPrevOut
	Value   uint64
	MinedAt uint32
	*BtcAddr
	destAddr string
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
	s = fmt.Sprintf("%15.8f  ", float64(ou.Value)/1e8) + ou.TxPrevOut.String()
	if ou.BtcAddr != nil {
		s += " " + ou.DestAddr() + ou.BtcAddr.Label()
	}
	if ou.MinedAt != 0 {
		s += fmt.Sprint("  ", ou.MinedAt)
	}
	return
}

func (ou *OneUnspentTx) UnspentTextLine() (s string) {
	s = fmt.Sprintf("%s # %.8f BTC @ %s%s, block %d", ou.TxPrevOut.String(),
		float64(ou.Value)/1e8, ou.DestAddr(), ou.BtcAddr.Label(), ou.MinedAt)
	return
}

func (ou *OneUnspentTx) DestAddr() string {
	if ou.destAddr == "" {
		return ou.BtcAddr.String()
	}
	return ou.destAddr
}
