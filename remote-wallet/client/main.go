package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

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
            if(strings.Compare(text, "no") != 0){
                fmt.Println("nooo")
                continue
            }
            txSignResp := common.Msg{}
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

