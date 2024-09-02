package main

import (
	"context"

	"github.com/piotrnar/gocoin/remote-wallet/common"
	"nhooyr.io/websocket"
	"nhooyr.io/websocket/wsjson"
)

type WalletRemoteGateway struct {
    c *websocket.Conn
}

func (wrc *WalletRemoteGateway) Open(addr string) (error) {
    ctx := context.Background()
	c, _, err := websocket.Dial(ctx, addr, nil)
	if err != nil {
        return err
	}
    wrc.c = c
    return err
}

func (wrc *WalletRemoteGateway) Close() error {
    return wrc.c.CloseNow()
}

func (wrc *WalletRemoteGateway) Write(msgType common.MsgType, payload interface{}) error {
    msg := common.Msg{Type: msgType, Payload: payload}
    err := wsjson.Write(context.Background(), wrc.c, msg)
    if err != nil {
        return err
    }
    return nil
}

func (wrc *WalletRemoteGateway) Read() (common.Msg, error) {
    var msg common.Msg
    err := wsjson.Read(context.Background(), wrc.c, &msg)
    if err != nil {
        return msg, err
    }
    return msg, nil
}
