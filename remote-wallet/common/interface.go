package common

type WalletRemoteServer interface {
    Start() error
    Stop()
}

type WalletRemoteClient interface {
    Open(string) error
    Close() error
    Read() (Msg, error)
    Write(MsgType, interface{}) error
}
