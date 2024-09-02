package main

import (
	"context"
	"fmt"
	"time"

	"github.com/piotrnar/gocoin/remote-wallet/common"
	"nhooyr.io/websocket"
	"nhooyr.io/websocket/wsjson"
)

type WSMessageWriter struct {
    conn *websocket.Conn
}

func (w WSMessageWriter) Write(msg common.Msg) error {
    ctx := context.Background()
    return wsjson.Write(ctx, w.conn, msg)
}

func main(){
    // initialize configuration
    common.InitConfig()
    fmt.Println("Establishing connection with ", common.FLAG.WebsocketServerAddr, "...")
    // establish connection with gocoin node 
    wrg := WalletRemoteGateway{}
    err := wrg.Open("ws://"+common.FLAG.WebsocketServerAddr)
	if err != nil {
        panic(err)
	}
    defer wrg.Close()
    // keep sending ping every 5 seconds so that the server can be aware of the connection
    writer := WSMessageWriter{conn: wrg.c}
    go func(){
        for range time.Tick(time.Second * 5) {
            ping := common.Msg{Type: common.Ping, Payload: nil}
            err = writer.Write(ping)
            if err != nil {
                fmt.Println(err)
                return 
            }
    }
    }()
    // process the requests from the gocoin node
    h := MsgHandler{WalletBinaryPath: common.FLAG.WalletFolderPath+"/wallet", WalletFolderPath: common.FLAG.WalletFolderPath}
    for {
        msg, err := wrg.Read()
        if err != nil {
            fmt.Println(err)
            break
        }
        switch msg.Type {
        case common.SignTransaction:
            txSignResp := common.Msg{}
            rawHex, err := h.SignTransaction(msg.Payload)
            if err != nil {
                fmt.Println(err)
                txSignResp.Type = common.InternalError
                txSignResp.Payload = common.SignTransactionRejectedError()
                writer.Write(txSignResp)
                continue
            }
            txSignResp.Type = common.SignedTransactionRawHex
            txSignResp.Payload = rawHex
            err = writer.Write(txSignResp)
            if err != nil {
                fmt.Println(err)
                break 
            }
        default:
            fmt.Println("Unknown message type")
    }
    }
}

