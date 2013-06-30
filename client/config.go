package main

import (
	"os"
	"fmt"
	"flag"
	"time"
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
	TXPool struct {
		Enabled bool // Global on/off swicth
		FeePerByte uint
		MaxTxSize uint
		MinVoutValue uint
		// If somethign is 1KB big, it expires after this many minutes.
		// Otherwise expiration time will be proportionally different.
		TxExpireMinPerKB uint
		TxExpireMaxHours uint
	}
	TXRoute struct {
		Enabled bool // Global on/off swicth
		FeePerByte uint
		MaxTxSize uint
		MinVoutValue uint
	}
	MeasureBlockTiming bool
}


func init() {
	// Fill in default values
	CFG.ListenTCP = true
	CFG.WebUI = "127.0.0.1:8833"

	CFG.TXPool.Enabled = true
	CFG.TXPool.FeePerByte = 0
	CFG.TXPool.MaxTxSize = 100e3
	CFG.TXPool.MinVoutValue = 0
	CFG.TXPool.TxExpireMinPerKB = 100
	CFG.TXPool.TxExpireMaxHours = 12

	CFG.TXRoute.Enabled = true
	CFG.TXRoute.FeePerByte = 10
	CFG.TXRoute.MaxTxSize = 10240
	CFG.TXRoute.MinVoutValue = 500*CFG.TXRoute.FeePerByte // Equivalent of 500 bytes tx fee

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
	flag.BoolVar(&CFG.TXRoute.Enabled, "txp", CFG.TXPool.Enabled, "Enable Memory Pool")
	flag.BoolVar(&CFG.TXRoute.Enabled, "txr", CFG.TXRoute.Enabled, "Enable Transaction Routing")

	if flag.Lookup("h") != nil {
		flag.PrintDefaults()
		os.Exit(0)
	}
	flag.Parse()

	MaxExpireTime = time.Duration(CFG.TXPool.TxExpireMaxHours) * time.Hour
	ExpirePerKB = time.Duration(CFG.TXPool.TxExpireMinPerKB) * time.Minute

	newUi("configsave cs", false, save_config, "Save current settings to a config file")
	newUi("configload cl", false, load_config, "Re-load settings from the config file")
	newUi("timing t", false, block_timing, "Switch block timing on/off")
}


func block_timing(s string) {
	if s=="1" || s=="on" {
		CFG.MeasureBlockTiming = true
	} else if s=="0" || s=="off" {
		CFG.MeasureBlockTiming = false
	} else {
		CFG.MeasureBlockTiming = !CFG.MeasureBlockTiming
	}
	fmt.Println("MeasureBlockTiming:", CFG.MeasureBlockTiming)
}


func load_config(s string) {
	d, e := ioutil.ReadFile(ConfigFile)
	if e != nil {
		println(e.Error())
		return
	}
	e = json.Unmarshal(d, &CFG)
	if e != nil {
		println(e.Error())
		return
	}
	MaxExpireTime = time.Duration(CFG.TXPool.TxExpireMaxHours) * time.Hour
	ExpirePerKB = time.Duration(CFG.TXPool.TxExpireMinPerKB) * time.Minute
	fmt.Println("Config reloaded")
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
