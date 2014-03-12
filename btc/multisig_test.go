package btc

import (
	"bytes"
	"testing"
	"encoding/hex"
)


func TestMultisigFromScript(t *testing.T) {
	txt := "004730440220485ef45dd67e7e3ffee699d42cf56ec88b4162d9f373770c30efec075468281702204929343ea97b007c1fc2ed49b306355ebf6bc5fb1613f0ed51ebca44fcc2003a014c69512103af88375d5fc9230446365b7d33540a73397ab3cc1a9f3e306a26833d1bfc260f21030677e0dd58025a5404747fbc64083040083acf3b390515f71a8ede95dc9c4d8a2103af88375d5fc9230446365b7d33540a73397ab3cc1a9f3e306a26833d1bfc260f53ae"
	d, _ := hex.DecodeString(txt)
	s, e := NewMultiSigFromScript(d)
	if e != nil {
		t.Error(e.Error())
	}

	b := s.Bytes()
	if !bytes.Equal(b, d) {
		t.Error("Multisig script does not match the input\n", hex.EncodeToString(b), "\n", hex.EncodeToString(d))
	}
}
