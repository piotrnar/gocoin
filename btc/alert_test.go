package btc

import (
	"testing"
	"encoding/hex"
)


func TestAlert(t *testing.T) {
	apk, _ := hex.DecodeString("04fc9702847840aaf195de8442ebecedf5b095cdbb9bc716bda9110971b28a49e0ead8564ff0db22209e0374782c093bb899692d524e9d6a6956e7c5ecbcd68284")
	dat, _ := hex.DecodeString("6d01000000506eb24f000000004c9e935100000000f7030000f50300000000000000979e000000881300000040555247454e543a20757067726164652072657175697265642c2073656520687474703a2f2f626974636f696e2e6f72672f646f7320666f722064657461696c7300483046022100c7994c5b0a8f5c84f714c54d30e251b55d5e9c733177fc81115375a6f7ca6910022100a93dc6e50cc58e512b29e522fb63a3428eabb2977930c2d4c6bfb9a2904533da")
	a, e := NewAlert(dat, apk)
	if e != nil {
		t.Error(e.Error())
	}
	if a.Version!=1 {
		t.Error("Incorrect version")
	}
	if a.RelayUntil!=0x4fb26e50 {
		t.Error("Incorrect RelayUntil")
	}
	if a.Expiration!=0x51939e4c {
		t.Error("Incorrect Expiration")
	}
	if a.ID!=0x000003f7 {
		t.Error("Incorrect ID")
	}
	if a.Cancel!=0x000003f5 {
		t.Error("Incorrect Cancel")
	}
	if a.MinVer!=0 {
		t.Error("Incorrect MinVer")
	}
	if a.MaxVer!=0x9e97 {
		t.Error("Incorrect MaxVer")
	}
	if a.Priority!=5000 {
		t.Error("Incorrect Priority")
	}
	if a.Comment!="" {
		t.Error("Incorrect Comment")
	}
	if a.StatusBar!="URGENT: upgrade required, see http://bitcoin.org/dos for details" {
		t.Error("Incorrect status bar message")
	}
	if a.Reserved!="" {
		t.Error("Incorrect Reserved")
	}
}
