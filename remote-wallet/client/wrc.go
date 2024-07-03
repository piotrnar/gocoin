package client

import (
	"context"

	"github.com/piotrnar/gocoin/remote-wallet/common"
	"nhooyr.io/websocket"
	"nhooyr.io/websocket/wsjson"
)

// WalletRemoteClient is used for interacting with the WalletRemoteServer. 
type WalletRemoteClient struct {
    c *websocket.Conn
}

func (wrc *WalletRemoteClient) Open(addr string) (error) {
    ctx := context.Background()
	c, _, err := websocket.Dial(ctx, addr, nil)
	if err != nil {
        return err
	}
    wrc.c = c
    return err
}

func (wrc *WalletRemoteClient) Close() error {
    return wrc.c.Close(websocket.StatusNormalClosure, "")
}

func (wrc *WalletRemoteClient) Write(msgType common.MsgType, payload interface{}) error {
    msg := common.Msg{Type: msgType, Payload: payload}
    err := wsjson.Write(context.Background(), wrc.c, msg)
    if err != nil {
        return err
    }
    return nil
}

func (wrc *WalletRemoteClient) Read() (common.Msg, error) {
    var msg common.Msg
    err := wsjson.Read(context.Background(), wrc.c, &msg)
    if err != nil {
        return msg, err
    }
    return msg, nil
}
