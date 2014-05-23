package common

import (
	"os"
	"fmt"
	"flag"
	"sync"
	"time"
	"strings"
	"io/ioutil"
	"runtime/debug"
	"encoding/json"
	"github.com/piotrnar/gocoin/lib/chain"
)

var (
	FLAG struct { // Command line only options
		Rescan bool
	}

	CFG struct { // Options that can come from either command line or common file
		Testnet bool
		ConnectOnly string
		Datadir string
		TextUI struct {
			Enabled bool
		}
		WebUI struct {
			Interface string
			AllowedIP string // comma separated
			ShowBlocks uint32
			AddrListLen uint32 // size of address list in MakeTx tab popups
		}
		Net struct {
			ListenTCP bool
			TCPPort uint16
			MaxOutCons uint32
			MaxInCons uint32
			MaxUpKBps uint
			MaxDownKBps uint
			MaxBlockAtOnce uint32
		}
		TXPool struct {
			Enabled bool // Global on/off swicth
			AllowMemInputs bool
			FeePerByte uint64
			MaxTxSize uint32
			MinVoutValue uint64
			// If something is 1KB big, it expires after this many minutes.
			// Otherwise expiration time will be proportionally different.
			TxExpireMinPerKB uint
			TxExpireMaxHours uint
		}
		TXRoute struct {
			Enabled bool // Global on/off swicth
			FeePerByte uint64
			MaxTxSize uint32
			MinVoutValue uint64
		}
		Memory struct {
			MinBrowsableVal uint64
			NoCacheBefore int // Use negetive value to not cache from the last block height plus it
			GCPercTrshold int
			MaxCachedBlocks uint
		}
		Beeps struct {
			NewBlock bool  // beep when a new block has been mined
			ActiveFork bool  // triple beep when there is a fork
			NewBalance bool // been when a balance has changed
			MinerID string // beep when a bew block is mined with this string in coinbase
		}
		MiningStatHours uint
		HashrateHours uint
		UserAgent string
		PayCommandName string
	}

	mutex_cfg sync.Mutex
)


type oneAllowedAddr struct {
	Addr, Mask uint32
}

var WebUIAllowed []oneAllowedAddr


func InitConfig() {
	// Fill in default values
	CFG.Net.ListenTCP = true
	CFG.Net.MaxOutCons = 9
	CFG.Net.MaxInCons = 10
	CFG.Net.MaxBlockAtOnce = 3

	CFG.TextUI.Enabled = true

	CFG.WebUI.Interface = "127.0.0.1:8833"
	CFG.WebUI.AllowedIP = "127.0.0.1"
	CFG.WebUI.ShowBlocks = 25
	CFG.WebUI.AddrListLen = 15

	CFG.TXPool.Enabled = true
	CFG.TXPool.AllowMemInputs = true
	CFG.TXPool.FeePerByte = 1
	CFG.TXPool.MaxTxSize = 10e3
	CFG.TXPool.MinVoutValue = 0
	CFG.TXPool.TxExpireMinPerKB = 180
	CFG.TXPool.TxExpireMaxHours = 12

	CFG.TXRoute.Enabled = true
	CFG.TXRoute.FeePerByte = 1
	CFG.TXRoute.MaxTxSize = 10e3
	CFG.TXRoute.MinVoutValue = 500*CFG.TXRoute.FeePerByte // Equivalent of 500 bytes tx fee

	CFG.Memory.GCPercTrshold = 100 // 100%
	CFG.Memory.MaxCachedBlocks = 500

	CFG.MiningStatHours = 24
	CFG.HashrateHours = 6
	CFG.UserAgent = DefaultUserAgent
	CFG.PayCommandName = "pay_cmd.txt"

	cfgfilecontent, e := ioutil.ReadFile(ConfigFile)
	if e==nil && len(cfgfilecontent)>0 {
		e = json.Unmarshal(cfgfilecontent, &CFG)
		if e != nil {
			println("Error in", ConfigFile, e.Error())
			os.Exit(1)
		}
	} else {
		// Create default config file
		SaveConfig()
		println("Stored default configuration in", ConfigFile)
	}

	flag.BoolVar(&FLAG.Rescan, "r", false, "Rebuild the unspent DB (fixes 'Unknown input TxID' errors)")
	flag.BoolVar(&CFG.Testnet, "t", CFG.Testnet, "Use Testnet3")
	flag.StringVar(&CFG.ConnectOnly, "c", CFG.ConnectOnly, "Connect only to this host and nowhere else")
	flag.BoolVar(&CFG.Net.ListenTCP, "l", CFG.Net.ListenTCP, "Listen for incoming TCP connections (on default port)")
	flag.StringVar(&CFG.Datadir, "d", CFG.Datadir, "Specify Gocoin's database root folder")
	flag.UintVar(&CFG.Net.MaxUpKBps, "ul", CFG.Net.MaxUpKBps, "Upload limit in KB/s (0 for no limit)")
	flag.UintVar(&CFG.Net.MaxDownKBps, "dl", CFG.Net.MaxDownKBps, "Download limit in KB/s (0 for no limit)")
	flag.StringVar(&CFG.WebUI.Interface, "webui", CFG.WebUI.Interface, "Serve WebUI from the given interface")
	flag.StringVar(&CFG.Beeps.MinerID, "miner", CFG.Beeps.MinerID, "Monitor new blocks with the string in their coinbase TX")
	flag.BoolVar(&CFG.TXRoute.Enabled, "txp", CFG.TXPool.Enabled, "Enable Memory Pool")
	flag.BoolVar(&CFG.TXRoute.Enabled, "txr", CFG.TXRoute.Enabled, "Enable Transaction Routing")
	flag.BoolVar(&CFG.TextUI.Enabled, "textui", CFG.TextUI.Enabled, "Enable processing TextUI commands (from stdin)")

	if flag.Lookup("h") != nil {
		flag.PrintDefaults()
		os.Exit(0)
	}
	flag.Parse()

	Reset()
}


func SaveConfig() bool {
	dat, _ := json.Marshal(&CFG)
	if dat == nil {
		return false
	}
	ioutil.WriteFile(ConfigFile, dat, 0660)
	return true

}

func Reset() {
	UploadLimit = CFG.Net.MaxUpKBps << 10
	DownloadLimit = CFG.Net.MaxDownKBps << 10
	debug.SetGCPercent(CFG.Memory.GCPercTrshold)
	MaxExpireTime = time.Duration(CFG.TXPool.TxExpireMaxHours) * time.Hour
	ExpirePerKB = time.Duration(CFG.TXPool.TxExpireMinPerKB) * time.Minute
	chain.NocacheBlocksBelow = CFG.Memory.NoCacheBefore
	chain.MinBrowsableOutValue = uint64(CFG.Memory.MinBrowsableVal)
	chain.MaxCachedBlocks = CFG.Memory.MaxCachedBlocks
	if CFG.Net.TCPPort != 0 {
		DefaultTcpPort = uint16(CFG.Net.TCPPort)
	} else {
		if CFG.Testnet {
			DefaultTcpPort = 18333
		} else {
			DefaultTcpPort = 8333
		}
	}

	ips := strings.Split(CFG.WebUI.AllowedIP, ",")
	WebUIAllowed = nil
	for i := range ips {
		oaa := str2oaa(ips[i])
		if oaa!=nil {
			WebUIAllowed = append(WebUIAllowed, *oaa)
		} else {
			println("ERROR: Incorrect AllowedIP:", ips[i])
		}
	}
	if len(WebUIAllowed)==0 {
		println("WARNING: No IP is currently allowed at WebUI")
	}
}


// Converts an IP range to addr/mask
func str2oaa(ip string) (res *oneAllowedAddr) {
	var a,b,c,d,x uint32
	n, _ := fmt.Sscanf(ip, "%d.%d.%d.%d/%d", &a, &b, &c, &d, &x)
	if n<4 {
		return
	}
	if (a|b|c|d)>255 || n==5 && (x<1 || x>32) {
		return
	}
	res = new(oneAllowedAddr)
	res.Addr = (a<<24) | (b<<16) | (c<<8) | d
	if n==4 || x==32 {
		res.Mask = 0xffffffff
	} else {
		res.Mask = uint32(( uint64(1) << (32-x) ) - 1)  ^ 0xffffffff
	}
	res.Addr &= res.Mask
	//fmt.Printf(" %s -> %08x / %08x\n", ip, res.Addr, res.Mask)
	return
}

func LockCfg() {
	mutex_cfg.Lock()
}

func UnlockCfg() {
	mutex_cfg.Unlock()
}

func CloseBlockChain() {
	if BlockChain!=nil {
		println("Closing blockchain")
		BlockChain.Sync()
		BlockChain.Save()
		BlockChain.Close()
		BlockChain = nil
	}
}
