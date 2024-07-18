package server

import (
	"fmt"

	"github.com/piotrnar/gocoin/remote-wallet/common"
)

type MessageWriter interface {
    Write(common.Msg) error
}

// MessageHandler contains all the logic for handling messages defined in the common.Msg package
type MsgHandler struct {
}

func(h *MsgHandler) ReceiveMessage(msg common.Msg, writer MessageWriter) {
    switch msg.Type {
    case common.SignedTransactionRawHex:
        fmt.Printf("Received transaction raw hex: %v\n", msg.Payload)
    default:
        fmt.Println("Unknown message type")
    }
}


