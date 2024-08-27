package common

import "flag"

var (
    FLAG struct { 
        WalletFolderPath string 
        WebsocketServerAddr string
    }
)

func InitConfig() {
	flag.StringVar(&FLAG.WalletFolderPath, "wallet", "/Users/humblenginr/code/gocoin/wallet", "Specify the path to the gocoin wallet directory")
    flag.StringVar(&FLAG.WebsocketServerAddr, "addr", "127.0.0.1:8878", "Specify the address of the gocoin node. It should be in the format <ip>:<port>")
}
