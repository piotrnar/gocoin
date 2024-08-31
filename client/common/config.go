package common

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"runtime/debug"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/piotrnar/gocoin"
	"github.com/piotrnar/gocoin/lib/btc"
	"github.com/piotrnar/gocoin/lib/others/sys"
	"github.com/piotrnar/gocoin/lib/utxo"
)

const LastTrustedBTCBlock = "00000000000000000001fcf207ce30e9172433f815bf4ca0e90ecd0601286a20" // #817490
const LastTrustedTN3Block = "00000000da84f2bafbbc53dee25a72ae507ff4914b867c565be350b0da8bf043" // #0

var (
	ConfigFile string = "gocoin.conf"

	FLAG struct { // Command line only options
		Rescan        bool
		VolatileUTXO  bool
		UndoBlocks    uint
		TrustAll      bool
		UnbanAllPeers bool
		NoWallet      bool
		Log           bool
		SaveConfig    bool
	}

	CFG struct { // Options that can come from either command line or common file
		Testnet          bool
		ConnectOnly      string
		Datadir          string
		TextUI_Enabled   bool
		UserAgent        string
		LastTrustedBlock string

		WebUI struct {
			Interface   string
			AllowedIP   string // comma separated
			ShowBlocks  uint32
			AddrListLen uint32 // size of address list in MakeTx tab popups
			Title       string
			PayCmdName  string
			ServerMode  bool
			SSLPort     uint16
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
			BindToIF       string
			MaxOutCons     uint32
			MaxInCons      uint32
			MaxUpKBps      uint
			MaxDownKBps    uint
			MaxBlockAtOnce uint32
			ExternalIP     string
		}
		TXPool struct {
			Enabled        bool // Global on/off swicth
			AllowMemInputs bool
			FeePerByte     float64
			MaxTxSize      uint32
			MaxSizeMB      uint
			MaxRejectMB    uint
			MaxRejectCnt   uint
			SaveOnDisk     bool
			Debug          bool
		}
		TXRoute struct {
			Enabled    bool // Global on/off swicth
			FeePerByte float64
			MaxTxSize  uint32
			MemInputs  bool
		}
		Memory struct {
			GCPercTrshold        int
			UseGoHeap            bool // Use Go Heap and Garbage Collector for UTXO records
			MaxCachedBlks        uint32
			FreeAtStart          bool // Free all possible memory after initial loading of block chain
			CacheOnDisk          bool
			MaxSyncCacheMB       uint32 // When syncing chain, prebuffer up to this MB of bocks data
			MaxDataFileMB        uint   // 0 for unlimited size
			DataFilesKeep        uint32 // 0 for all
			OldDataBackup        bool   // move old dat files to "oldat/" folder (instead of removing them)
			PurgeUnspendableUTXO bool
			CompressBlockDB      bool
		}
		AllBalances struct {
			MinValue  uint64 // Do not keep balance records for values lower than this
			UseMapCnt int
			AutoLoad  bool
		}
		Stat struct {
			HashrateHrs uint
			MiningHrs   uint
			FeesBlks    uint
			BSizeBlks   uint
			NoCounters  bool
		}
		DropPeers struct {
			DropEachMinutes uint // zero for never
			BlckExpireHours uint // zero for never
			PingPeriodSec   uint // zero to not ping
		}
		UTXOSave struct {
			SecondsToTake   uint   // zero for as fast as possible, 600 for do it in 10 minutes
			BlocksToHold    uint32 // zero for immediatelly, one for every other block...
			CompressRecords bool
		}
	}

	mutex_cfg sync.Mutex
)

type oneAllowedAddr struct {
	Addr, Mask uint32
}

var WebUIAllowed []oneAllowedAddr

func InitConfig() {
	var new_config_file bool

	// Fill in default values
	CFG.Net.ListenTCP = true
	CFG.Net.MaxOutCons = 9
	CFG.Net.MaxInCons = 10
	CFG.Net.MaxBlockAtOnce = 3
	CFG.Net.BindToIF = "0.0.0.0"

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
	CFG.TXPool.FeePerByte = 1.0
	CFG.TXPool.MaxTxSize = 100e3
	CFG.TXPool.MaxSizeMB = 300
	CFG.TXPool.MaxRejectMB = 25
	CFG.TXPool.MaxRejectCnt = 5000
	CFG.TXPool.SaveOnDisk = true

	CFG.TXRoute.Enabled = true
	CFG.TXRoute.FeePerByte = 0.0
	CFG.TXRoute.MaxTxSize = 100e3
	CFG.TXRoute.MemInputs = true

	CFG.Memory.GCPercTrshold = 30 // 30% (To save mem)
	CFG.Memory.MaxCachedBlks = 200
	CFG.Memory.CacheOnDisk = true
	CFG.Memory.MaxSyncCacheMB = 500
	CFG.Memory.MaxDataFileMB = 1000 // max 1GB per single data file
	CFG.Memory.CompressBlockDB = true

	CFG.Stat.HashrateHrs = 12
	CFG.Stat.MiningHrs = 24
	CFG.Stat.FeesBlks = 4 * 6 /*last 4 hours*/
	CFG.Stat.BSizeBlks = 1008 /*one week*/

	CFG.AllBalances.MinValue = 1e5 // 0.001 BTC
	CFG.AllBalances.UseMapCnt = 5000
	CFG.AllBalances.AutoLoad = true

	CFG.DropPeers.DropEachMinutes = 5  // minutes
	CFG.DropPeers.BlckExpireHours = 24 // hours
	CFG.DropPeers.PingPeriodSec = 15   // seconds

	CFG.UTXOSave.SecondsToTake = 300
	CFG.UTXOSave.BlocksToHold = 6

	if cfgfn := os.Getenv("GOCOIN_CLIENT_CONFIG"); cfgfn != "" {
		ConfigFile = cfgfn
	}
	// pre-parse command line: look for -cfg <fname> or -h
	for i := 1; i < len(os.Args); i++ {
		if os.Args[i] == "-cfg" || os.Args[i] == "--cfg" {
			if i+1 >= len(os.Args) {
				println("Missing the file name for", os.Args[i], "argument")
				os.Exit(1)
			}
			ConfigFile = os.Args[i+1]
			break
		}
		if strings.HasPrefix(os.Args[i], "-cfg=") || strings.HasPrefix(os.Args[i], "--cfg=") {
			ss := strings.SplitN(os.Args[i], "=", 2)
			ConfigFile = ss[1]
		}
	}

	cfgfilecontent, e := ioutil.ReadFile(ConfigFile)
	if e == nil && len(cfgfilecontent) > 0 {
		e = json.Unmarshal(cfgfilecontent, &CFG)
		if e != nil {
			println("Error in", ConfigFile, e.Error())
			os.Exit(1)
		}
		fmt.Println("Using config file", ConfigFile)
	} else {
		new_config_file = true
		fmt.Println("New config file", ConfigFile)
	}

	var _cfg_fn string
	flag.StringVar(&_cfg_fn, "cfg", ConfigFile, "Specify name of the config file")
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
	flag.BoolVar(&FLAG.NoWallet, "nowallet", FLAG.NoWallet, "Do not automatically enable the wallet functionality (lower memory usage and faster block processing)")
	flag.BoolVar(&FLAG.Log, "log", FLAG.Log, "Store some runtime information in the log files")
	flag.BoolVar(&FLAG.SaveConfig, "sc", FLAG.SaveConfig, "Save "+ConfigFile+" file and exit (use to create default config file)")

	if CFG.Datadir == "" {
		CFG.Datadir = sys.BitcoinHome() + "gocoin"
	}

	if flag.Lookup("h") != nil {
		flag.PrintDefaults()
		os.Exit(0)
	}
	flag.Parse()

	// swap LastTrustedBlock if it's now from the other chain
	if CFG.Testnet {
		if new_config_file || CFG.LastTrustedBlock == LastTrustedBTCBlock {
			CFG.LastTrustedBlock = LastTrustedTN3Block
		}
	} else {
		if new_config_file || CFG.LastTrustedBlock == LastTrustedTN3Block {
			CFG.LastTrustedBlock = LastTrustedBTCBlock
		}
	}

	ApplyBalMinVal()

	if !FLAG.NoWallet {
		if FLAG.UndoBlocks != 0 {
			FLAG.NoWallet = true // this will prevent loading of balances, thus speeding up the process
		} else {
			FLAG.NoWallet = !CFG.AllBalances.AutoLoad
		}
	}

	if new_config_file {
		// Create default config file
		// Set to purge unspndable UTXO records, for lower system memory usage
		CFG.Memory.PurgeUnspendableUTXO = true
		SaveConfig()
		println("Stored default configuration in", ConfigFile)
	}

	if CFG.Memory.UseGoHeap {
		fmt.Println("Using native Go heap with Garbage Collector for UTXO records")
	} else {
		fmt.Println("Using modernc.org/memory package to skip GC for UTXO records ")
		utxo.Memory_Malloc = func(le int) (res []byte) {
			MemMutex.Lock()
			res, _ = Memory.Malloc(le)
			MemMutex.Unlock()
			return
		}
		utxo.Memory_Free = func(ptr []byte) {
			MemMutex.Lock()
			Memory.Free(ptr)
			MemMutex.Unlock()
		}
	}

	if CFG.Memory.DataFilesKeep == 0 {
		Services |= btc.SERVICE_NETWORK
	}

	Reset()
}

func DataSubdir() string {
	if CFG.Testnet {
		return "ts4net"
	} else {
		return "btcnet"
	}
}

func SaveConfig() bool {
	dat, _ := json.MarshalIndent(&CFG, "", "    ")
	if dat == nil {
		return false
	}
	ioutil.WriteFile(ConfigFile, dat, 0660)
	return true

}

// make sure to call it with locked mutex_cfg
func Reset() {
	SetUploadLimit(uint64(CFG.Net.MaxUpKBps) << 10)
	SetDownloadLimit(uint64(CFG.Net.MaxDownKBps) << 10)
	debug.SetGCPercent(CFG.Memory.GCPercTrshold)
	if AllBalMinVal() != CFG.AllBalances.MinValue {
		fmt.Println("In order to apply the new value of AllBalMinVal, restart the node or do 'wallet off' and 'wallet on'")
	}
	DropSlowestEvery = time.Duration(CFG.DropPeers.DropEachMinutes) * time.Minute
	BlockExpireEvery = time.Duration(CFG.DropPeers.BlckExpireHours) * time.Hour
	PingPeerEvery = time.Duration(CFG.DropPeers.PingPeriodSec) * time.Second

	atomic.StoreUint64(&maxMempoolSizeBytes, uint64(CFG.TXPool.MaxSizeMB)*1e6)
	atomic.StoreUint64(&maxRejectedSizeBytes, uint64(CFG.TXPool.MaxRejectMB)*1e6)
	atomic.StoreUint64(&minFeePerKB, uint64(CFG.TXPool.FeePerByte*1000))
	atomic.StoreUint64(&minminFeePerKB, MinFeePerKB())

	if CFG.Memory.MaxSyncCacheMB < 100 {
		CFG.Memory.MaxSyncCacheMB = 100
	}
	SyncMaxCacheBytes.Store(int(CFG.Memory.MaxSyncCacheMB) << 20)

	if CFG.Stat.NoCounters {
		if !NoCounters.Get() {
			// switching counters off - reset the data
			NoCounters.Set()
			CounterMutex.Lock()
			Counter = make(map[string]uint64)
			CounterMutex.Unlock()
		}
	} else {
		NoCounters.Clr()
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
	ListenTCP = CFG.Net.ListenTCP

	utxo.UTXO_WRITING_TIME_TARGET = time.Second * time.Duration(CFG.UTXOSave.SecondsToTake)
	utxo.UTXO_SKIP_SAVE_BLOCKS = CFG.UTXOSave.BlocksToHold
	utxo.UTXO_PURGE_UNSPENDABLE = CFG.Memory.PurgeUnspendableUTXO

	if CFG.UserAgent != "" {
		UserAgent = CFG.UserAgent
	} else {
		UserAgent = "/Gocoin:" + gocoin.Version + "/"
	}

	if CFG.Memory.MaxDataFileMB != 0 && CFG.Memory.MaxDataFileMB < 8 {
		CFG.Memory.MaxDataFileMB = 8
	}

	if CFG.Net.BindToIF == "" {
		CFG.Net.BindToIF = "0.0.0.0"
	}

	MkTempBlocksDir()

	ReloadMiners()

	ApplyLastTrustedBlock()
}

func MkTempBlocksDir() {
	// no point doing it before GocoinHomeDir is set in host_init()
	if CFG.Memory.CacheOnDisk && GocoinHomeDir != "" {
		os.Mkdir(TempBlocksDir(), 0700)
	}
}

func RPCPort() (res uint32) {
	mutex_cfg.Lock()
	defer mutex_cfg.Unlock()

	if CFG.RPC.TCPPort != 0 {
		res = CFG.RPC.TCPPort
		return
	}
	if CFG.Testnet {
		res = 18332
	} else {
		res = 8332
	}
	return
}

func DefaultTcpPort() (res uint16) {
	mutex_cfg.Lock()
	defer mutex_cfg.Unlock()

	if CFG.Net.TCPPort != 0 {
		res = CFG.Net.TCPPort
		return
	}
	if CFG.Testnet {
		res = 48333
	} else {
		res = 8333
	}
	return
}

// str2oaa converts an IP range to addr/mask
func str2oaa(ip string) (res *oneAllowedAddr) {
	var a, b, c, d, x uint32
	n, _ := fmt.Sscanf(ip, "%d.%d.%d.%d/%d", &a, &b, &c, &d, &x)
	if n < 4 {
		return
	}
	if (a|b|c|d) > 255 || n == 5 && x > 32 {
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
		//BlockChain = nil
	}
}

func GetDuration(addr *time.Duration) (res time.Duration) {
	mutex_cfg.Lock()
	res = *addr
	mutex_cfg.Unlock()
	return
}

func GetUint64(addr *uint64) (res uint64) {
	mutex_cfg.Lock()
	res = *addr
	mutex_cfg.Unlock()
	return
}

func GetUint32(addr *uint32) (res uint32) {
	mutex_cfg.Lock()
	res = *addr
	mutex_cfg.Unlock()
	return
}

func SetUint32(addr *uint32, val uint32) {
	mutex_cfg.Lock()
	*addr = val
	mutex_cfg.Unlock()
}

func GetBool(addr *bool) (res bool) {
	mutex_cfg.Lock()
	res = *addr
	mutex_cfg.Unlock()
	return
}

func SetBool(addr *bool, val bool) {
	mutex_cfg.Lock()
	*addr = val
	mutex_cfg.Unlock()
}

func AllBalMinVal() uint64 {
	return atomic.LoadUint64(&allBalMinVal)
}

func ApplyBalMinVal() {
	atomic.StoreUint64(&allBalMinVal, CFG.AllBalances.MinValue)
}

func MinFeePerKB() uint64 {
	return atomic.LoadUint64(&minFeePerKB)
}

func SetMinFeePerKB(val uint64) bool {
	minmin := atomic.LoadUint64(&minminFeePerKB)
	if val < minmin {
		val = minmin
	}
	if val == MinFeePerKB() {
		return false
	}
	atomic.StoreUint64(&minFeePerKB, val)
	return true
}

func RouteMinFeePerKB() uint64 {
	return atomic.LoadUint64(&routeMinFeePerKB)
}

func IsListenTCP() (res bool) {
	mutex_cfg.Lock()
	res = CFG.ConnectOnly == "" && ListenTCP
	mutex_cfg.Unlock()
	return
}

func MaxMempoolSize() uint64 {
	return atomic.LoadUint64(&maxMempoolSizeBytes)
}

func RejectedTxsLimits() (size uint64, cnt int) {
	mutex_cfg.Lock()
	size = maxRejectedSizeBytes
	cnt = int(CFG.TXPool.MaxRejectCnt)
	mutex_cfg.Unlock()
	return
}

func TempBlocksDir() string {
	return GocoinHomeDir + "tmpblk" + string(os.PathSeparator)
}

func GetExternalIp() (res string) {
	mutex_cfg.Lock()
	res = CFG.Net.ExternalIP
	mutex_cfg.Unlock()
	return
}
