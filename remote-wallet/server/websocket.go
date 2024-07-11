package server

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/piotrnar/gocoin/remote-wallet/common"
	"nhooyr.io/websocket"
	"nhooyr.io/websocket/wsjson"
)

const (
    // amount of time in seconds to wait before considering the connection to be broken 
    PingWaitPeriod = 10
)

type WSMessageWriter struct {
    conn *websocket.Conn
}

func (w WSMessageWriter) Write(msg common.Msg) error {
    ctx := context.Background()
    return wsjson.Write(ctx, w.conn, msg)
}

type WebsocketServer struct {
    handler *MsgHandler
    Conn *websocket.Conn

    Signtxchan (chan common.Msg)
    Pingchan (chan common.Msg)
}

func NewWebsocketServer(msg *MsgHandler) WebsocketServer {
    signtxchan := make(chan common.Msg, 0)
    pingchan := make(chan common.Msg, 0)
    return WebsocketServer{msg, nil, signtxchan, pingchan}
}

func(s *WebsocketServer) Start(addr string) error {
    l, err := net.Listen("tcp", addr)
	if err != nil {
		panic(err)
	}
	fmt.Printf("listening on ws://%v\n", l.Addr())
	server := &http.Server{
		Handler: s,
	}
    if err != nil {
        return err
    }
    errc := make(chan error, 1)
	go func() {
		errc <- server.Serve(l)
	}()
	err = <-errc
    fmt.Printf("failed to serve: %v", err)
	return nil
}

func(s *WebsocketServer) ConnectionStatus() bool {
    if(s.Conn == nil){
        return false
    }
    return true
}

func (s *WebsocketServer) handleMessages(){
    for {
        msg := common.Msg{}
        error := wsjson.Read(context.Background(), s.Conn, &msg)
        if error != nil {
            fmt.Println(error)
            // error while reading can only be because of a corrupt connection
            return 
        }
        switch msg.Type {
            case common.Ping:
                s.Pingchan <- msg
            case common.SignedTransactionRawHex:
                s.Signtxchan <- msg
        }
}
}

func (s *WebsocketServer) watchPings(){
    ticker := time.NewTicker(PingWaitPeriod * time.Second)
    for {
        select {
        case <-s.Pingchan:
            fmt.Println("Received Ping")
            ticker.Reset(PingWaitPeriod * time.Second)
        case <-ticker.C:
            // harsh attempt to clean up resources
            s.Conn.CloseNow()
            s.Conn = nil
            return
        }
    }
}


func (s *WebsocketServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
    if(s.Conn != nil) {
      http.Error(w, "Websocket connection already exists", http.StatusForbidden)
        return
    }
	c, err := websocket.Accept(w, r, nil)
	if err != nil {
        fmt.Println(err.Error())
		return  
	}
    s.Conn = c

    go s.handleMessages()
    go s.watchPings()

    fmt.Println("Websocket connection established with the wallet remote client")
}

func(s *WebsocketServer) Stop() {}
