package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/piotrnar/gocoin/remote-wallet/common"
	"nhooyr.io/websocket"
	"nhooyr.io/websocket/wsjson"
)

var(
    TempWalletFolderPath = "/Users/humblenginr/code/gocoin/wallet"
    TempWalletBinaryPath = TempWalletFolderPath + "/wallet"
    WebsockerServerAddr = "127.0.0.1:8878"

)

type WSMessageWriter struct {
    conn *websocket.Conn
}

func (w WSMessageWriter) Write(msg common.Msg) error {
    ctx := context.Background()
    return wsjson.Write(ctx, w.conn, msg)
}

func main(){
    if(len(os.Args) == 2) {
        WebsockerServerAddr = os.Args[1]
    }
    fmt.Println("Establishing connection with ", WebsockerServerAddr, "...")
    wrc := WalletRemoteClient{}
    err := wrc.Open("ws://"+WebsockerServerAddr)
	if err != nil {
        panic(err)
	}
    h := MsgHandler{WalletBinaryPath: TempWalletBinaryPath, WalletFolderPath: TempWalletFolderPath}
    writer := WSMessageWriter{conn: wrc.c}

    go func(){
        // keep sending ping every 5 seconds so that the server can be aware of the connection
        for range time.Tick(time.Second * 5) {
            ping := common.Msg{Type: common.Ping, Payload: nil}
            err = writer.Write(ping)
            if err != nil {
                fmt.Println(err)
                return 
            }
    }
    }()

    for {
        msg, err := wrc.Read()
        if err != nil {
            fmt.Println(err)
            break
        }
        switch msg.Type {
        case common.SignTransaction:
            reader := bufio.NewReader(os.Stdin)
            fmt.Print("Received a request to sign a transaction. Do you want to confirm(yes/no): ")
            text, _ := reader.ReadString('\n')
            txSignResp := common.Msg{}
            if strings.TrimRight(text, "\n") == "no" {
                txSignResp.Type = common.InternalError
                txSignResp.Payload = common.SignTransactionRejectedError()
                writer.Write(txSignResp)
                continue
            }
            rawHex, err := h.SignTransaction(msg.Payload)
            if err != nil {
                fmt.Println(err)
                return
            }
            txSignResp.Type = common.SignedTransactionRawHex
            txSignResp.Payload = rawHex
            err = writer.Write(txSignResp)
            if err != nil {
                fmt.Println(err)
                return 
            }
        default:
            fmt.Println("Unknown message type")
    }

    }
}

