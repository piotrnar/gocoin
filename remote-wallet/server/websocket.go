package server

import (
	"context"
	"fmt"
	"net"
	"net/http"

	"nhooyr.io/websocket"
	"nhooyr.io/websocket/wsjson"
	"github.com/piotrnar/gocoin/remote-wallet/common"
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
}

func NewWebsocketServer(msg *MsgHandler) WebsocketServer {
    return WebsocketServer{msg, nil}
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

func (s *WebsocketServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	c, err := websocket.Accept(w, r, nil)
	if err != nil {
        fmt.Println(err.Error())
		return  
	}
    s.Conn = c
    fmt.Println("Websocket connection establish with the wallet remote client")
}

func(s *WebsocketServer) Stop() {}
