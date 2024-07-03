package common

// this is how a server should be
type WalletRemoteServer interface {
    Start() error
    Stop()
}

// this is how a client should be
type WalletRemoteClient interface {
    Open(string) error
    Close() error
    Read() (Msg, error)
    Write(MsgType, interface{}) error
}
