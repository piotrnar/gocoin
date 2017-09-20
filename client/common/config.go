package common

import (
	"encoding/json"
	"flag"
	"fmt"
	"github.com/piotrnar/gocoin"
	"github.com/piotrnar/gocoin/lib/others/sys"
	"github.com/piotrnar/gocoin/lib/utxo"
	"io/ioutil"
	"os"
	"runtime/debug"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

var (
	FLAG struct { // Command line only options
		Rescan        bool
		VolatileUTXO  bool
		UndoBlocks    uint
		TrustAll      bool
		UnbanAllPeers bool
		NoWallet      bool
		Log           bool
	}

	CFG struct { // Options that can come from either command line or common file
		Testnet        bool
		ConnectOnly    string
		Datadir        string
		TextUI_Enabled bool
		UserAgent      string
		UTXOSaveSec    uint
		WebUI          struct {
			Interface   string
			AllowedIP   string // comma separated
			ShowBlocks  uint32
			AddrListLen uint32 // size of address list in MakeTx tab popups
			Title       string
			PayCmdName  string
			ServerMode  bool
		}
		RPC struct {
			Enabled  bool
			Username string
			Password string
			TCPPort  uint32
		}
		Net struct {
			ListenTCP      bool
			TCPPort        uint16
			MaxOutCons     uint32
			MaxInCons      uint32
			MaxUpKBps      uint
			MaxDownKBps    uint
			MaxBlockAtOnce uint32
			MinSegwitCons  uint32
		}
		TXPool struct {
			Enabled        bool // Global on/off swicth
			AllowMemInputs bool
			FeePerByte     uint64
			MaxTxSize      uint32
			// If something is 1KB big, it expires after this many minutes.
			// Otherwise expiration time will be proportionally different.
			ExpireMinPerKB uint
			ExpireMaxHours uint
			SaveOnDisk     bool
		}
		TXRoute struct {
			Enabled    bool // Global on/off swicth
			FeePerByte uint64
			MaxTxSize  uint32
		}
		Memory struct {
			GCPercTrshold int
			MaxCachedBlks uint
			FreeAtStart   bool // Free all possible memory after initial loading of block chain
		}
		AllBalances struct {
			MinValue   uint64 // Do not keep balance records for values lower than this
			UseMapCnt  int
			SaveOnDisk bool
		}
		Stat struct {
			HashrateHrs uint
			MiningHrs   uint
			FeesBlks    uint
			BSizeBlks   uint
		}
		DropPeers struct {
			DropEachMinutes uint // zero for never
			BlckExpireHours uint // zero for never
			PingPeriodSec   uint // zero to not ping
		}
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
	CFG.Net.MinSegwitCons = 4

	CFG.TextUI_Enabled = true

	CFG.WebUI.Interface = "127.0.0.1:8833"
	CFG.WebUI.AllowedIP = "127.0.0.1"
	CFG.WebUI.ShowBlocks = 144
	CFG.WebUI.AddrListLen = 15
	CFG.WebUI.Title = "Gocoin"
	CFG.WebUI.PayCmdName = "pay_cmd.txt"

	CFG.RPC.Username = "gocoinrpc"
	CFG.RPC.Password = "gocoinpwd"

	CFG.TXPool.Enabled = true
	CFG.TXPool.AllowMemInputs = true
	CFG.TXPool.FeePerByte = 20
	CFG.TXPool.MaxTxSize = 100e3
	CFG.TXPool.ExpireMinPerKB = 1800
	CFG.TXPool.ExpireMaxHours = 120

	CFG.TXRoute.Enabled = true
	CFG.TXRoute.FeePerByte = 25
	CFG.TXRoute.MaxTxSize = 100e3

	CFG.Memory.GCPercTrshold = 100 // 100% (Go's default)
	CFG.Memory.MaxCachedBlks = 200

	CFG.Stat.HashrateHrs = 12
	CFG.Stat.MiningHrs = 24
	CFG.Stat.FeesBlks = 4 * 6   /*last 4 hours*/
	CFG.Stat.BSizeBlks = 12 * 6 /*half a day*/

	CFG.AllBalances.MinValue = 1e5 // 0.001 BTC
	CFG.AllBalances.UseMapCnt = 100

	CFG.DropPeers.DropEachMinutes = 5  // minutes
	CFG.DropPeers.BlckExpireHours = 24 // hours
	CFG.DropPeers.PingPeriodSec = 15   // seconds

	cfgfilecontent, e := ioutil.ReadFile(ConfigFile)
	if e == nil && len(cfgfilecontent) > 0 {
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

	flag.BoolVar(&FLAG.Rescan, "r", false, "Rebuild UTXO database (fixes 'Unknown input TxID' errors)")
	flag.BoolVar(&FLAG.VolatileUTXO, "v", false, "Use UTXO database in volatile mode (speeds up rebuilding)")
	flag.BoolVar(&CFG.Testnet, "t", CFG.Testnet, "Use Testnet3")
	flag.StringVar(&CFG.ConnectOnly, "c", CFG.ConnectOnly, "Connect only to this host and nowhere else")
	flag.BoolVar(&CFG.Net.ListenTCP, "l", CFG.Net.ListenTCP, "Listen for incoming TCP connections (on default port)")
	flag.StringVar(&CFG.Datadir, "d", CFG.Datadir, "Specify Gocoin's database root folder")
	flag.UintVar(&CFG.Net.MaxUpKBps, "ul", CFG.Net.MaxUpKBps, "Upload limit in KB/s (0 for no limit)")
	flag.UintVar(&CFG.Net.MaxDownKBps, "dl", CFG.Net.MaxDownKBps, "Download limit in KB/s (0 for no limit)")
	flag.StringVar(&CFG.WebUI.Interface, "webui", CFG.WebUI.Interface, "Serve WebUI from the given interface")
	flag.BoolVar(&CFG.TXRoute.Enabled, "txp", CFG.TXPool.Enabled, "Enable Memory Pool")
	flag.BoolVar(&CFG.TXRoute.Enabled, "txr", CFG.TXRoute.Enabled, "Enable Transaction Routing")
	flag.BoolVar(&CFG.TextUI_Enabled, "textui", CFG.TextUI_Enabled, "Enable processing TextUI commands (from stdin)")
	flag.UintVar(&FLAG.UndoBlocks, "undo", 0, "Undo UTXO with this many blocks and exit")
	flag.BoolVar(&FLAG.TrustAll, "trust", FLAG.TrustAll, "Trust all scripts inside new blocks (for fast syncig)")
	flag.BoolVar(&FLAG.UnbanAllPeers, "unban", FLAG.UnbanAllPeers, "Un-ban all peers in databse, before starting")
	flag.BoolVar(&FLAG.NoWallet, "nowallet", FLAG.NoWallet, "Do not monitor balances (saves time and memory, use to sync chain)")
	flag.BoolVar(&FLAG.Log, "log", FLAG.Log, "Store some runtime information in the log files")

	if CFG.Datadir == "" {
		CFG.Datadir = sys.BitcoinHome() + "gocoin"
	}

	if flag.Lookup("h") != nil {
		flag.PrintDefaults()
		os.Exit(0)
	}
	flag.Parse()

	AllBalMinVal = CFG.AllBalances.MinValue

	if FLAG.UndoBlocks != 0 {
		FLAG.NoWallet = true // this will prevent loading of balances, thus speeding up the process
	}

	Reset()
}

func DataSubdir() string {
	if CFG.Testnet {
		return "tstnet"
	} else {
		return "btcnet"
	}
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
	MaxExpireTime = time.Duration(CFG.TXPool.ExpireMaxHours) * time.Hour
	ExpirePerKB = time.Duration(CFG.TXPool.ExpireMinPerKB) * time.Minute
	if AllBalMinVal != CFG.AllBalances.MinValue {
		fmt.Println("In order to apply the new value of AllBalMinVal, node's restart is requirted")
	}
	DropSlowestEvery = time.Duration(CFG.DropPeers.DropEachMinutes) * time.Minute
	BlockExpireEvery = time.Duration(CFG.DropPeers.BlckExpireHours) * time.Hour
	PingPeerEvery = time.Duration(CFG.DropPeers.PingPeriodSec) * time.Second
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
		if oaa != nil {
			WebUIAllowed = append(WebUIAllowed, *oaa)
		} else {
			println("ERROR: Incorrect AllowedIP:", ips[i])
		}
	}
	if len(WebUIAllowed) == 0 {
		println("WARNING: No IP is currently allowed at WebUI")
	}
	SetListenTCP(CFG.Net.ListenTCP, false)

	if CFG.UTXOSaveSec != 0 {
		utxo.UTXO_WRITING_TIME_TARGET = time.Second * time.Duration(CFG.UTXOSaveSec)
	}

	connect_only = CFG.ConnectOnly != ""

	if CFG.UserAgent != "" {
		UserAgent = CFG.UserAgent
	} else {
		UserAgent = "/Gocoin:" + gocoin.Version + "/"
	}

	ReloadMiners()
}

func RPCPort() uint32 {
	if CFG.RPC.TCPPort != 0 {
		return CFG.RPC.TCPPort
	}
	if CFG.Testnet {
		return 18332
	} else {
		return 8332
	}
}

// Converts an IP range to addr/mask
func str2oaa(ip string) (res *oneAllowedAddr) {
	var a, b, c, d, x uint32
	n, _ := fmt.Sscanf(ip, "%d.%d.%d.%d/%d", &a, &b, &c, &d, &x)
	if n < 4 {
		return
	}
	if (a|b|c|d) > 255 || n == 5 && (x < 1 || x > 32) {
		return
	}
	res = new(oneAllowedAddr)
	res.Addr = (a << 24) | (b << 16) | (c << 8) | d
	if n == 4 || x == 32 {
		res.Mask = 0xffffffff
	} else {
		res.Mask = uint32((uint64(1)<<(32-x))-1) ^ 0xffffffff
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
	if BlockChain != nil {
		fmt.Println("Closing BlockChain")
		BlockChain.Close()
		BlockChain = nil
	}
}

var listen_tcp uint32
var connect_only bool // set in Reset()

func IsListenTCP() bool {
	return !connect_only && atomic.LoadUint32(&listen_tcp) != 0
}

func SetListenTCP(yes bool, global bool) {
	if yes {
		atomic.StoreUint32(&listen_tcp, 1)
	} else {
		atomic.StoreUint32(&listen_tcp, 0)
	}
	if global {
		// Make sure mutex_cfg is locked while calling this one
		CFG.Net.ListenTCP = yes
	}
}
