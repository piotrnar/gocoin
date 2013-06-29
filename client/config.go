package main

import (
	"os"
	"fmt"
	"flag"
	"io/ioutil"
	"encoding/json"
)


const ConfigFile = "gocoin.conf"

// Here are command line only options
var FLAG struct {
	rescan bool
}

// Here are options that can come from either command line or config file
var CFG struct {
	Testnet bool
	ConnectOnly string
	ListenTCP bool
	Datadir string
	Nosync bool
	MaxUpKBps uint
	MaxDownKBps uint
	WebUI string
	MinerID string
	TXRouting struct {
		Enabled bool // Global on/off swicth
		FeePerByte uint
		MaxTxSize uint
		MinVoutValue uint

		// If somethign is 1KB big, it expires after this many minutes.
		// Otherwise expiration time will be proportionally different.
		TxExpirePerKB uint
	}
	MeasureBlockTiming bool
}


func init() {
	// Fill in default values
	CFG.ListenTCP = true
	CFG.WebUI = "127.0.0.1:8833"
	CFG.TXRouting.Enabled = true
	CFG.TXRouting.FeePerByte = 10
	CFG.TXRouting.MaxTxSize = 10240
	CFG.TXRouting.MinVoutValue = 500*CFG.TXRouting.FeePerByte // Equivalent of 500 bytes tx fee
	CFG.TXRouting.TxExpirePerKB = 120 // Two hours

	cfgfilecontent, e := ioutil.ReadFile(ConfigFile)
	if e == nil {
		e = json.Unmarshal(cfgfilecontent, &CFG)
		if e != nil {
			println("Error in", ConfigFile, e.Error())
			os.Exit(1)
		}
	}

	flag.BoolVar(&FLAG.rescan, "r", false, "Rebuild the unspent DB (fixes 'Unknown input TxID' errors)")
	flag.BoolVar(&CFG.Testnet, "t", CFG.Testnet, "Use Testnet3")
	flag.StringVar(&CFG.ConnectOnly, "c", CFG.ConnectOnly, "Connect only to this host and nowhere else")
	flag.BoolVar(&CFG.ListenTCP, "l", CFG.ListenTCP, "Listen for incomming TCP connections (on default port)")
	flag.StringVar(&CFG.Datadir, "d", CFG.Datadir, "Specify Gocoin's database root folder")
	flag.BoolVar(&CFG.Nosync, "nosync", CFG.Nosync, "Init blockchain with syncing disabled (dangerous!)")
	flag.UintVar(&CFG.MaxUpKBps, "ul", CFG.MaxUpKBps, "Upload limit in KB/s (0 for no limit)")
	flag.UintVar(&CFG.MaxDownKBps, "dl", CFG.MaxDownKBps, "Download limit in KB/s (0 for no limit)")
	flag.StringVar(&CFG.WebUI, "webui", CFG.WebUI, "Serve WebUI from the given interface")
	flag.StringVar(&CFG.MinerID, "miner", CFG.MinerID, "Monitor new blocks with the string in their coinbase TX")
	flag.BoolVar(&CFG.TXRouting.Enabled, "txr", CFG.TXRouting.Enabled, "Enable Transaction Routing")

	if flag.Lookup("h") != nil {
		flag.PrintDefaults()
		os.Exit(0)
	}
	flag.Parse()

	newUi("saveconfig sc", false, save_config, "Save current settings to a config file")
}


func save_config(s string) {
	CFG.MaxUpKBps = UploadLimit>>10
	CFG.MaxDownKBps = DownloadLimit>>10
	dat, _ := json.Marshal(&CFG)
	if dat != nil {
		ioutil.WriteFile(ConfigFile, dat, 0660)
		fmt.Println("Current settings saved to", ConfigFile)
	}
}
