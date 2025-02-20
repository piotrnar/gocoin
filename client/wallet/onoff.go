package wallet

import (
	"bytes"
	"encoding/gob"
	"os"
	"unsafe"

	"github.com/piotrnar/gocoin/client/common"
	"github.com/piotrnar/gocoin/lib/utxo"
)

var (
	FetchingBalanceTick func() bool
	OnOff               chan bool = make(chan bool, 1)
)

func InitMaps(empty bool) {
	var szs [IDX_CNT]int

	if !empty {
		LoadMapSizes()
		szs = WalletAddrsCount[common.AllBalMinVal()]
		// If yet unknown, just continue with zero size maps
	}

	for i := range AllBalances {
		AllBalances[i] = make(map[string]*OneAllAddrBal, szs[i])
	}
}

func LoadBalancesFromUtxo() {
	if common.Get(&common.WalletON) {
		//fmt.Println("wallet.LoadBalance() ignore: ", common.GetBool(&common.WalletON))
		return
	}

	common.Set(&common.WalletProgress, 1)
	common.ApplyBalMinVal()

	InitMaps(false)

	var prv_key = -1
	var aborted bool
	common.BlockChain.Unspent.Browse(func(k, v []byte) bool {
		TxNotifyAdd(utxo.NewUtxoRecStatic(*(*utxo.UtxoKeyType)(unsafe.Pointer(&k[0])), v))
		if FetchingBalanceTick != nil && FetchingBalanceTick() {
			aborted = true
			return true
		}
		if int(k[0]) != prv_key {
			prv_key = int(k[0])
			common.Set(&common.WalletProgress, uint32(1000*prv_key/256))
		}
		return aborted
	})

	if aborted {
		InitMaps(true)
	} else {
		common.BlockChain.Unspent.CB.NotifyTxAdd = TxNotifyAdd
		common.BlockChain.Unspent.CB.NotifyTxDel = TxNotifyDel
		common.Set(&common.WalletON, true)
	}
	common.Set(&common.WalletProgress, 0)
}

func Disable() {
	if !common.Get(&common.WalletON) {
		//fmt.Println("wallet.Disable() ignore: ", common.GetBool(&common.WalletON))
		return
	}
	UpdateMapSizes()
	common.BlockChain.Unspent.CB.NotifyTxAdd = nil
	common.BlockChain.Unspent.CB.NotifyTxDel = nil
	common.Set(&common.WalletON, false)
	InitMaps(true)
}

const (
	MAPSIZ_FILE_NAME = "mapsize.gob"
)

var (
	WalletAddrsCount map[uint64][IDX_CNT]int = make(map[uint64][IDX_CNT]int) //index:MinValue, [0]-P2KH, [1]-P2SH, [2]-P2WSH, [3]-P2WKH, [4]-P2TAP
)

func UpdateMapSizes() {
	var tmp [IDX_CNT]int
	for i := range tmp {
		tmp[i] = len(AllBalances[i])
	}
	WalletAddrsCount[common.AllBalMinVal()] = tmp
	buf := new(bytes.Buffer)
	gob.NewEncoder(buf).Encode(WalletAddrsCount)
	os.WriteFile(common.GocoinHomeDir+MAPSIZ_FILE_NAME, buf.Bytes(), 0600)
}

func LoadMapSizes() {
	d, er := os.ReadFile(common.GocoinHomeDir + MAPSIZ_FILE_NAME)
	if er != nil {
		println("LoadMapSizes:", er.Error())
		return
	}

	buf := bytes.NewBuffer(d)

	er = gob.NewDecoder(buf).Decode(&WalletAddrsCount)
	if er != nil {
		println("LoadMapSizes:", er.Error())
	}
}
