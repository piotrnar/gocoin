package common


type WalletRemoteServer interface {
    Start(string) error
    Stop()
    ConnectionStatus() bool
}

type WalletRemoteClient interface {
    Open(string) error
    Close() error
    Read() (Msg, error)
    Write(MsgType, interface{}) error
}
