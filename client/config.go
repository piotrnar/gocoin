package main

import (
	"os"
	"fmt"
	"flag"
	"time"
	"io/ioutil"
	"runtime/debug"
	"encoding/json"
	"github.com/piotrnar/gocoin/btc/qdb"
)


const ConfigFile = "gocoin.conf"

// Here are command line only options
var FLAG struct {
	rescan bool
	nosync bool
}

// Here are options that can come from either command line or config file
var CFG struct {
	Testnet bool
	ConnectOnly string
	Datadir string
	WebUI string
	MinerID string
	Net struct {
		ListenTCP bool
		TCPPort uint
		MaxOutCons uint
		MaxInCons uint
		MaxUpKBps uint
		MaxDownKBps uint
	}
	TXPool struct {
		Enabled bool // Global on/off swicth
		AllowMemInputs bool
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
	Memory struct {
		MinBrowsableVal uint
		NoCacheBefore uint
		GCPercTrshold int
	}
}


func init() {
	// Fill in default values
	CFG.Net.ListenTCP = true
	CFG.Net.MaxOutCons = 8
	CFG.Net.MaxInCons = 8
	CFG.WebUI = "127.0.0.1:8833"

	CFG.TXPool.Enabled = true
	CFG.TXPool.AllowMemInputs = true
	CFG.TXPool.FeePerByte = 10
	CFG.TXPool.MaxTxSize = 10e3
	CFG.TXPool.MinVoutValue = 0
	CFG.TXPool.TxExpireMinPerKB = 100
	CFG.TXPool.TxExpireMaxHours = 12

	CFG.TXRoute.Enabled = true
	CFG.TXRoute.FeePerByte = 10
	CFG.TXRoute.MaxTxSize = 10e3
	CFG.TXRoute.MinVoutValue = 500*CFG.TXRoute.FeePerByte // Equivalent of 500 bytes tx fee

	CFG.Memory.GCPercTrshold = 100 // 100%

	cfgfilecontent, e := ioutil.ReadFile(ConfigFile)
	if e == nil {
		e = json.Unmarshal(cfgfilecontent, &CFG)
		if e != nil {
			println("Error in", ConfigFile, e.Error())
			os.Exit(1)
		}
	}

	flag.BoolVar(&FLAG.rescan, "r", false, "Rebuild the unspent DB (fixes 'Unknown input TxID' errors)")
	flag.BoolVar(&FLAG.nosync, "nosync", false, "Init blockchain with syncing disabled (dangerous!)")
	flag.BoolVar(&CFG.Testnet, "t", CFG.Testnet, "Use Testnet3")
	flag.StringVar(&CFG.ConnectOnly, "c", CFG.ConnectOnly, "Connect only to this host and nowhere else")
	flag.BoolVar(&CFG.Net.ListenTCP, "l", CFG.Net.ListenTCP, "Listen for incomming TCP connections (on default port)")
	flag.StringVar(&CFG.Datadir, "d", CFG.Datadir, "Specify Gocoin's database root folder")
	flag.UintVar(&CFG.Net.MaxUpKBps, "ul", CFG.Net.MaxUpKBps, "Upload limit in KB/s (0 for no limit)")
	flag.UintVar(&CFG.Net.MaxDownKBps, "dl", CFG.Net.MaxDownKBps, "Download limit in KB/s (0 for no limit)")
	flag.StringVar(&CFG.WebUI, "webui", CFG.WebUI, "Serve WebUI from the given interface")
	flag.StringVar(&CFG.MinerID, "miner", CFG.MinerID, "Monitor new blocks with the string in their coinbase TX")
	flag.BoolVar(&CFG.TXRoute.Enabled, "txp", CFG.TXPool.Enabled, "Enable Memory Pool")
	flag.BoolVar(&CFG.TXRoute.Enabled, "txr", CFG.TXRoute.Enabled, "Enable Transaction Routing")

	if flag.Lookup("h") != nil {
		flag.PrintDefaults()
		os.Exit(0)
	}
	flag.Parse()

	resetcfg()

	newUi("configsave cs", false, save_config, "Save current settings to a config file")
	newUi("configload cl", false, load_config, "Re-load settings from the config file")
	newUi("configset cfg", false, set_config, "Set a specific config value - use JSON, omit top {}")
}


func resetcfg() {
	UploadLimit = CFG.Net.MaxUpKBps << 10
	DownloadLimit = CFG.Net.MaxDownKBps << 10
	debug.SetGCPercent(CFG.Memory.GCPercTrshold)
	MaxExpireTime = time.Duration(CFG.TXPool.TxExpireMaxHours) * time.Hour
	ExpirePerKB = time.Duration(CFG.TXPool.TxExpireMinPerKB) * time.Minute
	qdb.NocacheBlocksBelow = CFG.Memory.NoCacheBefore
	qdb.MinBrowsableOutValue = uint64(CFG.Memory.MinBrowsableVal)
	if CFG.Net.TCPPort != 0 {
		DefaultTcpPort = uint16(CFG.Net.TCPPort)
	} else {
		if CFG.Testnet {
			DefaultTcpPort = 18333
		} else {
			DefaultTcpPort = 8333
		}
	}
}


func set_config(s string) {
	if s!="" {
		new := CFG
		e := json.Unmarshal([]byte("{"+s+"}"), &new)
		if e != nil {
			println(e.Error())
		} else {
			CFG = new
			resetcfg()
			fmt.Println("Config changed. Execute configsave, if you want to save it.")
		}
	}
	dat, _ := json.Marshal(&CFG)
	fmt.Println(string(dat))
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
	resetcfg()
	fmt.Println("Config reloaded")
}


func save_config(s string) {
	dat, _ := json.Marshal(&CFG)
	if dat != nil {
		ioutil.WriteFile(ConfigFile, dat, 0660)
		fmt.Println("Current settings saved to", ConfigFile)
	}
}
