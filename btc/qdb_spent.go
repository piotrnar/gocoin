package btc

import (
	"io"
	"encoding/binary"
)


/*
The spent value:
 [0] - 1-added / 0 - deleted
 [1:33] - TxPrevOut.Hash
 [33:37] - TxPrevOut.Vout LSB
 These only for delted:
  [37:45] - Value
  [45:49] - PK_Script length
  [49:] - PK_Script
 [X:X+4] - crc32
*/



func writeSpent(f io.Writer, po *TxPrevOut, to *TxOut) {
	if to == nil {
		// added
		f.Write([]byte{1})
		f.Write(po.Hash[:])
		binary.Write(f, binary.LittleEndian, uint32(po.Vout))
	} else {
		// deleted
		f.Write([]byte{0})
		f.Write(po.Hash[:])
		binary.Write(f, binary.LittleEndian, uint32(po.Vout))
		binary.Write(f, binary.LittleEndian, uint64(to.Value))
		binary.Write(f, binary.LittleEndian, uint32(len(to.Pk_script)))
		f.Write(to.Pk_script[:])
	}
}


func readSpent(f io.Reader) (po *TxPrevOut, to *TxOut) {
	var buf [49]byte
	n, e := f.Read(buf[:37])
	if n!=37 || e!=nil || buf[0]>1 {
		return
	}
	po = new(TxPrevOut)
	copy(po.Hash[:], buf[1:33])
	po.Vout = binary.LittleEndian.Uint32(buf[33:37])
	if buf[0]==0 {
		n, e = f.Read(buf[37:49])
		if n!=12 || e!=nil {
			panic("Unexpected end of file")
		}
		to = new(TxOut)
		to.Value = binary.LittleEndian.Uint64(buf[37:45])
		to.Pk_script = make([]byte, binary.LittleEndian.Uint32(buf[45:49]))
		f.Read(to.Pk_script[:])
	}
	return
}
