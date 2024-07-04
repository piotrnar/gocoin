package client

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/piotrnar/gocoin/remote-wallet/common"
)

// WaitAndEstablishConnection runs a http server at common.ClientTcpServerAddr and waits for a message of type common.InitiateConnection.
// After receiving the message, it attempts to establish a websocket connection with the WalletRemoteServer.
func WaitAndEstablishConnection(wrc *WalletRemoteClient) {
    var websocketServerAddr string
    initCon := make(chan bool)
    mux := http.NewServeMux()
    srv := http.Server{Addr: common.ClientTcpServerAddr, Handler: mux}
    mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
        var msg common.Msg
        body, err := io.ReadAll(r.Body)
        if err != nil {
            fmt.Println(err)
            return 
        }
        err = json.Unmarshal(body, &msg)
        if err != nil {
            fmt.Println(err)
            return 
        }
        if(msg.Type != common.InitiateConnection) {
            fmt.Println("Unknown message type encountered!")
            return
        }
        fmt.Println("Received initate connection request")
        websocketServerAddr = msg.Payload.(string)
        w.Write([]byte("ok"))
        initCon <- true
    })
     
    go func(){
        fmt.Println("HTTP server running at: ", common.ClientTcpServerAddr)
        err := srv.ListenAndServe()
        if err != nil && err != http.ErrServerClosed {
            panic(err)
        }
    }()
    <-initCon
    fmt.Println("Shutting down the temp tcp server")
    srv.Close()

    err := wrc.Open(websocketServerAddr)
	if err != nil {
        panic(err)
	}
    fmt.Println("Websocket connection established with the Remote Wallet Server")
}
