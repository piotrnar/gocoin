package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/signal"

	"github.com/piotrnar/gocoin/remote-wallet/common"
)

var (
    TempWalletFolderPath = "/Users/humblenginr/code/gocoin/wallet"
    TempWalletBinaryPath = TempWalletFolderPath + "/wallet"
    WebsocketServerAddr = "127.0.0.1:8878"
)

/* sendInitiateConnectionReq sends a message of type common.InitiateConnection to the WalletRemoteClient so that it can initiate the conneciton. This is to improve the security of the system by not allowing anyone to be able to connect to the wallet (which contains valuable information).
The current infrastructure is not secure by any means and serves only as a basement for building a secure system. This whole procedure shall be abstracted away in the future and made more secure. 
*/
func sendInitiateConnectionReq() {
    var data = common.Msg{Type: common.InitiateConnection, Payload: "ws://"+WebsocketServerAddr}
    marshalled, err := json.Marshal(data)
    if err != nil {
        fmt.Printf("impossible to marshall: %s", err)
    }

    url := "http://"+common.ClientTcpServerAddr + "/"
	req, err := http.NewRequest("POST", url, bytes.NewReader(marshalled))
    if err != nil {
        fmt.Printf("impossible to build request: %s", err)
    }
    req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	response, error := client.Do(req)
	if error != nil {
		panic(error)
	}
    fmt.Println("InitiateConnection request sent")
	defer response.Body.Close()
}

func main() {
    if(len(os.Args) == 4) {
        TempWalletBinaryPath = os.Args[2]
        TempWalletFolderPath = os.Args[3]
    } 
    go func(){
        msgHandler := NewHandler(TempWalletFolderPath, TempWalletBinaryPath)
        wsServer := NewWebsocketServer(&msgHandler)
        wsServer.Start(WebsocketServerAddr)
    }()
    sendInitiateConnectionReq()

    sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, os.Interrupt)
    <- sigs
    fmt.Println("Received signal to terminate the program...")
    return 
}
