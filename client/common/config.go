package common

import (
	"encoding/json"
	"flag"
	"fmt"
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

const LastTrustedBTCBlock = "0000000000000000000034bfd467dccafd796ae5367942c55cdb85dae2cd9be8" // #931178
const LastTrustedTN3Block = "00000000000000fa9c23f20506e6c57b6dda928fb2110629bf5d29df2f737ad2" // #3800000
const LastTrustedTN4Block = "00000000000000014b648b97c5361a3ba9c5f6db88afc1810f5d59fb7d557c12" // #117401

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
		NoMempoolLoad bool
	}

	CFG struct { // Options that can come from either command line or common file
		Testnet          bool
		Testnet4         bool
		ConnectOnly      string
		Datadir          string
		UtxoSubdir       string
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
			MaxTxWeight    uint32
			MaxSizeMB      uint
			ExpireInDays   uint
			MaxRejectMB    float64
			MaxNoUtxoMB    float64
			RejectRecCnt   uint16
			SaveOnDisk     bool
			NotFullRBF     bool
			//CheckForErrors bool
		}
		TXRoute struct {
			Enabled     bool // Global on/off swicth
			FeePerByte  float64
			MaxTxWeight uint32
			MemInputs   bool
		}
		Memory struct {
			GCPercTrshold        int
			MemoryLimitMB        uint32
			UseGoHeap            bool // Use Go Heap and Garbage Collector for UTXO records
			MaxCachedBlks        uint32
			CacheOnDisk          bool
			SyncCacheSize        uint32 // When syncing chain, prebuffer up to this MB of bocks data
			MaxDataFileMB        uint   // 0 for unlimited size
			DataFilesKeep        uint32 // 0 for all
			OldDataBackup        bool   // move old dat files to "oldat/" folder (instead of removing them)
			PurgeUnspendableUTXO bool
			CompressBlockDB      bool
		}
		AllBalances struct {
			MinValue      uint64 // Do not keep balance records for values lower than this
			UseMapCnt     uint32
			AutoLoad      bool
			SaveBalances  bool
			InstantWallet bool
		}
		Stat struct {
			HashrateHrs uint
			MiningHrs   uint
			FeesBlks    uint
			BSizeBlks   uint
			NoCounters  bool
		}
		DropPeers struct {
			DropEachMinutes uint32 // zero for never
			BlckExpireHours uint32 // zero for never
			PingPeriodSec   uint32 // zero to not ping
			ImmunityMinutes uint32
		}
		UTXOSave struct {
			SecondsToTake   uint   // zero for as fast as possible, 600 for do it in 10 minutes
			BlocksToHold    uint32 // zero for immediatelly, one for every other block...
			CompressRecords bool
		}
	}

	mutex_cfg sync.Mutex

	defaultMemoryLimit = debug.SetMemoryLimit(-1)
)

type oneAllowedAddr struct {
	Addr, Mask uint32
}

var WebUIAllowed []oneAllowedAddr

func AssureValueInRange[T int | uint | uint16 | uint32 | float64](label string, val *T, min, max T) {
	if max < min {
		panic("max < min")
	}
	nval := *val
	if *val < min {
		nval = min
	} else if *val > max {
		nval = max
	}
	if nval != *val {
		fmt.Println("WARNING: config value", label, "must be from", min, "to", max, " so changinig", *val, "->", nval)
		*val = nval
	}
}

func InitConfig() {
	var new_config_file bool

	// Fill in default values
	CFG.Net.ListenTCP = true
	CFG.Net.MaxOutCons = 10
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
	CFG.TXPool.FeePerByte = 0.001
	CFG.TXPool.MaxTxWeight = 400e3
	CFG.TXPool.MaxSizeMB = 500
	CFG.TXPool.ExpireInDays = 14
	CFG.TXPool.MaxRejectMB = 25.0
	CFG.TXPool.MaxNoUtxoMB = 5.0
	CFG.TXPool.RejectRecCnt = 20000
	CFG.TXPool.SaveOnDisk = true

	CFG.TXRoute.Enabled = true
	CFG.TXRoute.FeePerByte = 0.1
	CFG.TXRoute.MaxTxWeight = 400e3
	CFG.TXRoute.MemInputs = false

	CFG.Memory.GCPercTrshold = 10 // 30% (To save mem)
	CFG.Memory.MaxCachedBlks = 200
	CFG.Memory.CacheOnDisk = true
	CFG.Memory.SyncCacheSize = 500
	CFG.Memory.MaxDataFileMB = 1000 // max 1GB per single data file
	CFG.Memory.CompressBlockDB = true

	CFG.Stat.HashrateHrs = 12
	CFG.Stat.MiningHrs = 24
	CFG.Stat.FeesBlks = 4 * 6 /*last 4 hours*/
	CFG.Stat.BSizeBlks = 1008 /*one week*/

	CFG.AllBalances.MinValue = 1e5 // 0.001 BTC
	CFG.AllBalances.UseMapCnt = 5000
	CFG.AllBalances.AutoLoad = true
	CFG.AllBalances.SaveBalances = true
	CFG.AllBalances.InstantWallet = false

	CFG.DropPeers.DropEachMinutes = 5  // minutes
	CFG.DropPeers.BlckExpireHours = 24 // hours
	CFG.DropPeers.PingPeriodSec = 15   // seconds
	CFG.DropPeers.ImmunityMinutes = 15

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

	cfgfilecontent, e := os.ReadFile(ConfigFile)
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
	var testnet3, testnet4 bool
	flag.StringVar(&_cfg_fn, "cfg", ConfigFile, "Specify name of the config file")
	flag.BoolVar(&FLAG.Rescan, "r", false, "Rebuild UTXO database (fixes 'Unknown input TxID' errors)")
	flag.BoolVar(&FLAG.VolatileUTXO, "v", false, "Use UTXO database in volatile mode (speeds up rebuilding)")
	flag.BoolVar(&testnet4, "t", CFG.Testnet && CFG.Testnet4, "Use Testnet4")
	flag.BoolVar(&testnet3, "t3", CFG.Testnet && !CFG.Testnet4, "Use Testnet3")
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
	flag.BoolVar(&FLAG.NoMempoolLoad, "mp0", FLAG.NoMempoolLoad, "Do not attempt to load mempool from disk (start with empty one)")
	flag.BoolVar(&CFG.AllBalances.InstantWallet, "iw", CFG.AllBalances.InstantWallet, "Make sure to fetch all wallet balances before starting UI and network")

	if CFG.Datadir == "" {
		CFG.Datadir = sys.BitcoinHome() + "gocoin"
	}

	if flag.Lookup("h") != nil {
		flag.PrintDefaults()
		os.Exit(0)
	}
	flag.Parse()

	CFG.Testnet = testnet3 || testnet4
	CFG.Testnet4 = testnet4

	// swap LastTrustedBlock if it's now from the other chain
	if CFG.Testnet {
		if CFG.Testnet4 {
			if new_config_file || CFG.LastTrustedBlock == LastTrustedBTCBlock || CFG.LastTrustedBlock == LastTrustedTN4Block {
				CFG.LastTrustedBlock = LastTrustedTN4Block
			}
		} else {
			if new_config_file || CFG.LastTrustedBlock == LastTrustedBTCBlock || CFG.LastTrustedBlock == LastTrustedTN3Block {
				CFG.LastTrustedBlock = LastTrustedTN3Block
			}
		}
	} else {
		if new_config_file || CFG.LastTrustedBlock == LastTrustedTN3Block || CFG.LastTrustedBlock == LastTrustedTN4Block {
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
		MemoryModUsed = true
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
		if CFG.Testnet4 {
			return "ts4net"
		} else {
			return "tstnet"
		}
	} else {
		return "btcnet"
	}
}

func SaveConfig() bool {
	dat, _ := json.MarshalIndent(&CFG, "", "    ")
	if dat == nil {
		return false
	}
	os.WriteFile(ConfigFile, dat, 0660)
	return true

}

// make sure to call it with locked mutex_cfg
func Reset() {
	SetUploadLimit(uint64(CFG.Net.MaxUpKBps) << 10)
	SetDownloadLimit(uint64(CFG.Net.MaxDownKBps) << 10)
	if CFG.Memory.GCPercTrshold < -1 {
		fmt.Println("INFO: GCPercTrshold config value changed from", CFG.Memory.GCPercTrshold, "to -1")
		CFG.Memory.GCPercTrshold = -1
	} else if CFG.Memory.GCPercTrshold == 0 {
		fmt.Println("INFO: GCPercTrshold config value changed from", CFG.Memory.GCPercTrshold, "to 1")
		CFG.Memory.GCPercTrshold = 1
	}
	debug.SetGCPercent(CFG.Memory.GCPercTrshold)
	UpdateMemoryLimit()
	if AllBalMinVal() != CFG.AllBalances.MinValue {
		fmt.Println("In order to apply the new value of AllBalMinVal, restart the node or do 'wallet off' and 'wallet on'")
	}
	DropSlowestEvery = time.Duration(CFG.DropPeers.DropEachMinutes) * time.Minute
	BlockExpireEvery = time.Duration(CFG.DropPeers.BlckExpireHours) * time.Hour
	PingPeerEvery = time.Duration(CFG.DropPeers.PingPeriodSec) * time.Second
	AssureValueInRange("TXPool.ExpireInDays", &CFG.TXPool.ExpireInDays, 1, 1e6)
	TxExpireAfter = time.Duration(CFG.TXPool.ExpireInDays) * time.Hour * 24

	if CFG.TXPool.MaxSizeMB > 0 {
		AssureValueInRange("TXPool.MaxSizeMB", &CFG.TXPool.MaxSizeMB, 10, 1e6)
		atomic.StoreUint64(&maxMempoolSizeBytes, uint64(float64(CFG.TXPool.MaxSizeMB)*1e6))
	} else {
		fmt.Println("WARNING: TXPool config value MaxSizeMB is zero (unlimited mempool size)")
	}
	AssureValueInRange("TXPool.RejectRecCnt", &CFG.TXPool.RejectRecCnt, 100, 60000)
	if CFG.TXPool.MaxRejectMB != 0 {
		AssureValueInRange("TXPool.MaxRejectMB", &CFG.TXPool.MaxRejectMB, 0.3, 1e6)
		atomic.StoreUint64(&MaxRejectedSizeBytes, uint64(CFG.TXPool.MaxRejectMB*1e6))
	} else {
		fmt.Println("WARNING: TXPool config value MaxRejectMB is zero (unlimited rejected txs cache size)")
	}
	if CFG.TXPool.MaxNoUtxoMB == 0 {
		atomic.StoreUint64(&MaxNoUtxoSizeBytes, atomic.LoadUint64(&MaxRejectedSizeBytes))
	} else if CFG.TXPool.MaxRejectMB != 0 && CFG.TXPool.MaxNoUtxoMB > CFG.TXPool.MaxRejectMB {
		fmt.Println("WARNING: TXPool config value MaxNoUtxoMB not smaller then MaxRejectMB (ignoring it)")
		atomic.StoreUint64(&MaxNoUtxoSizeBytes, atomic.LoadUint64(&MaxRejectedSizeBytes))
	} else {
		AssureValueInRange("TXPool.MaxNoUtxoMB", &CFG.TXPool.MaxNoUtxoMB, 0.1, 1e6)
		atomic.StoreUint64(&MaxNoUtxoSizeBytes, uint64(CFG.TXPool.MaxNoUtxoMB*1e6))
	}
	atomic.StoreUint64(&minFeePerKB, uint64(CFG.TXPool.FeePerByte*1000))
	atomic.StoreUint64(&cfgFeePerKB, MinFeePerKB())

	atomic.StoreUint64(&cfgRouteMinFeePerKB, uint64(CFG.TXRoute.FeePerByte*1000))

	if CFG.Memory.SyncCacheSize < 100 {
		CFG.Memory.SyncCacheSize = 100
	}
	if CFG.Memory.CacheOnDisk {
		SyncMaxCacheBytes.Store(int(CFG.Memory.SyncCacheSize) << 23) // 8x bigger cache if on disk (500MB -> 4GB)
	} else {
		SyncMaxCacheBytes.Store(int(CFG.Memory.SyncCacheSize) << 20)
	}

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

// Mind that this is called with config mutex locked.
func UpdateMemoryLimit() {
	if CFG.Memory.MemoryLimitMB != 0 {
		MemMutex.Lock()
		utxo_bytes_used := int64(Memory.Bytes)
		MemMutex.Unlock()
		new_limit := int64(CFG.Memory.MemoryLimitMB) << 20
		if new_limit > utxo_bytes_used {
			debug.SetMemoryLimit(new_limit - utxo_bytes_used)
			if warningShown {
				debug.SetGCPercent(CFG.Memory.GCPercTrshold)
				warningShown = false
			}
		} else {
			debug.SetMemoryLimit(0)
			debug.SetGCPercent(1)
			if !warningShown {
				fmt.Println("WARNING: Disabled MemoryLimitMB as it is too low:", new_limit, "<", utxo_bytes_used)
				fmt.Println(" Called SetGCPercent(1) instead")
				warningShown = true
			}
		}
	} else {
		debug.SetMemoryLimit(defaultMemoryLimit)
	}
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
	} else {
		res = uint32(DefaultTcpPort) - 1
	}
	return
}

func ConfiguredTcpPort() (res uint16) {
	mutex_cfg.Lock()
	defer mutex_cfg.Unlock()

	if CFG.Net.TCPPort != 0 {
		res = CFG.Net.TCPPort
	} else {
		res = DefaultTcpPort
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
		fmt.Println("Closing BlockChain at block", BlockChain.LastBlock().Height)
		BlockChain.Close()
		//BlockChain = nil
	}
}

func Get[T bool | uint16 | uint32 | uint64 | time.Duration](addr *T) (res T) {
	mutex_cfg.Lock()
	res = *addr
	mutex_cfg.Unlock()
	return
}

func Set[T bool | uint16 | uint32 | int64 | uint64 | time.Duration](addr *T, val T) {
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
	cfgmin := atomic.LoadUint64(&cfgFeePerKB)
	if val < cfgmin {
		val = cfgmin // do not set it lower than the value from the config
	}
	if val == MinFeePerKB() {
		return false
	}
	atomic.StoreUint64(&minFeePerKB, val)
	return true
}

func RouteMinFeePerKB() uint64 {
	return atomic.LoadUint64(&cfgRouteMinFeePerKB)
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

func TempBlocksDir() string {
	return GocoinHomeDir + "tmpblk" + string(os.PathSeparator)
}

func GetExternalIp() (res string) {
	mutex_cfg.Lock()
	res = CFG.Net.ExternalIP
	mutex_cfg.Unlock()
	return
}
