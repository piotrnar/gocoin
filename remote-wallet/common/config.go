package common

import "flag"

var (
    FLAG struct { 
        WalletFolderPath string 
        RemoteWalletServerAddr string
    }
)

func InitConfig() {
	flag.StringVar(&FLAG.WalletFolderPath, "wallet", "", "Specify the path to the gocoin wallet directory. A temp directory is created by default.")
    flag.StringVar(&FLAG.RemoteWalletServerAddr, "addr", "127.0.0.1:8878", "Specify the address of the gocoin node. It should be in the format <ip>:<port>")
    flag.Parse()
}
