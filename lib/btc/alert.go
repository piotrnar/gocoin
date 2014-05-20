package btc

import (
	"os"
	"bytes"
	"errors"
	"encoding/binary"
)

type Alert struct {
	Version uint32    // Alert format version
	RelayUntil int64  // The timestamp beyond which nodes should stop relaying this alert
	Expiration int64  // The timestamp beyond which this alert is no longer in effect and should be ignored
	ID int32          // A unique ID number for this alert
	Cancel int32      // All alerts with an ID number less than or equal to this number should be canceled: deleted and not accepted in the future
	SetCancel []int32 // All alert IDs contained in this set should be canceled as above
	MinVer int32      // This alert only applies to versions greater than or equal to this version. Other versions should still relay it.
	MaxVer int32      // This alert only applies to versions less than or equal to this version. Other versions should still relay it.
	SetSubVer[]string // If this set contains any elements, then only nodes that have their subVer contained in this set are affected by the alert. Other versions should still relay it.
	Priority int32    // Relative priority compared to other alerts
	Comment string    // A comment on the alert that is not displayed
	StatusBar string  // The alert message that is displayed to the user
	Reserved string   // Reserved
}


func NewAlert(b []byte, alertPubKey []byte) (res *Alert, e error) {
	var le uint64
	rd := bytes.NewReader(b)

	// read payload
	tmp, e := ReadString(rd)
	if e != nil {
		e = errors.New("NewAlert: error reading payload")
		return
	}
	payload := []byte(tmp)

	// read signature
	tmp, e = ReadString(rd)
	if e != nil {
		e = errors.New("NewAlert: error reading signature")
		return
	}
	signature := []byte(tmp)

	h := NewSha2Hash(payload)
	if !EcdsaVerify(alertPubKey, signature, h.Hash[:]) {
		e = errors.New("NewAlert: The signature is not correct")
		return
	}

	rd = bytes.NewReader(payload)
	res = new(Alert)

	if e = binary.Read(rd, binary.LittleEndian, &res.Version); e != nil {
		return
	}
	if e = binary.Read(rd, binary.LittleEndian, &res.RelayUntil); e != nil {
		return
	}
	if e = binary.Read(rd, binary.LittleEndian, &res.Expiration); e != nil {
		return
	}
	if e = binary.Read(rd, binary.LittleEndian, &res.ID); e != nil {
		return
	}
	if e = binary.Read(rd, binary.LittleEndian, &res.Cancel); e != nil {
		return
	}

	if le, e = ReadVLen(rd); e != nil {
		return
	}
	if le > 0 {
		res.SetCancel = make([]int32, le)
		for i := 0; i < int(le); i++ {
			if e = binary.Read(rd, binary.LittleEndian, &res.SetCancel[i]); e != nil {
				return
			}
		}
	}
	rd.Seek(int64(le), os.SEEK_CUR) // skip SetCancel

	if e = binary.Read(rd, binary.LittleEndian, &res.MinVer); e != nil {
		return
	}
	if e = binary.Read(rd, binary.LittleEndian, &res.MaxVer); e != nil {
		return
	}


	if le, e = ReadVLen(rd); e != nil {
		return
	}
	if le > 0 {
		res.SetSubVer = make([]string, le)
		for i := 0; i < int(le); i++ {
			if res.SetSubVer[i], e = ReadString(rd); e != nil {
				return
			}
		}
	}

	if e = binary.Read(rd, binary.LittleEndian, &res.Priority); e != nil {
		return
	}

	if res.Comment, e = ReadString(rd); e != nil {
		return
	}

	if res.StatusBar, e = ReadString(rd); e != nil {
		return
	}

	if res.Reserved, e = ReadString(rd); e != nil {
		return
	}

	return
}
