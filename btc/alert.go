package btc

import (
	"os"
	"bytes"
	"errors"
	"encoding/hex"
	"encoding/binary"
)

type Alert struct {
	Version uint32    // Alert format version
	RelayUntil int64  // The timestamp beyond which nodes should stop relaying this alert
	Expiration int64  // The timestamp beyond which this alert is no longer in effect and should be ignored
	ID int32          // A unique ID number for this alert
	Cancel int32      // All alerts with an ID number less than or equal to this number should be canceled: deleted and not accepted in the future
	//setCancel set<int> // All alert IDs contained in this set should be canceled as above
	MinVer int32      // This alert only applies to versions greater than or equal to this version. Other versions should still relay it.
	MaxVer int32      // This alert only applies to versions less than or equal to this version. Other versions should still relay it.
	// setSubVer set<string> // If this set contains any elements, then only nodes that have their subVer contained in this set are affected by the alert. Other versions should still relay it.
	Priority int32    // Relative priority compared to other alerts
	Comment string    // A comment on the alert that is not displayed
	StatusBar string  // The alert message that is displayed to the user
	Reserved string   // Reserved
}

var alertPubKey []byte


func NewAlert(b []byte) (res *Alert, e error) {
	var payload, signature []byte
	var le uint64

	rd := bytes.NewReader(b)

	// read payload
	le, e = ReadVLen(rd)
	if e != nil {
		return
	}
	payload = make([]byte, le)
	_, e = rd.Read(payload)
	if e != nil {
		return
	}

	// read signature
	le, e = ReadVLen(rd)
	if e != nil {
		return
	}
	signature = make([]byte, le)
	_, e = rd.Read(signature)
	if e != nil {
		return
	}

	h := NewSha2Hash(payload)
	if !EcdsaVerify(alertPubKey, signature, h.Hash[:]) {
		e = errors.New("The alert's signature is not correct")
		return
	}

	rd = bytes.NewReader(payload)
	res = new(Alert)
	binary.Read(rd, binary.LittleEndian, &res.Version)
	binary.Read(rd, binary.LittleEndian, &res.RelayUntil)
	binary.Read(rd, binary.LittleEndian, &res.Expiration)
	binary.Read(rd, binary.LittleEndian, &res.ID)
	binary.Read(rd, binary.LittleEndian, &res.Cancel)

	le, e = ReadVLen(rd)
	if e != nil {
		return
	}
	rd.Seek(int64(le), os.SEEK_CUR) // skip setCancel

	binary.Read(rd, binary.LittleEndian, &res.MinVer)
	binary.Read(rd, binary.LittleEndian, &res.MaxVer)

	le, e = ReadVLen(rd)
	if e != nil {
		return
	}
	rd.Seek(int64(le), os.SEEK_CUR) // skip setCancel

	binary.Read(rd, binary.LittleEndian, &res.Priority)
	res.Comment, e = ReadString(rd)
	res.StatusBar, e = ReadString(rd)
	res.Reserved, e = ReadString(rd)
	return
}


func init() {
	alertPubKey, _ = hex.DecodeString("04fc9702847840aaf195de8442ebecedf5b095cdbb9bc716bda9110971b28a49e0ead8564ff0db22209e0374782c093bb899692d524e9d6a6956e7c5ecbcd68284")
}
