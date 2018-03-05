package wallet

import (
	"bytes"
	"encoding/gob"
	"github.com/piotrnar/gocoin/client/common"
	"github.com/piotrnar/gocoin/lib/utxo"
	"io/ioutil"
)

var (
	FetchingBalanceTick func() bool
	OnOff               chan bool = make(chan bool, 1)
)

func InitMaps(empty bool) {
	var szs [4]int
	var ok bool

	if empty {
		goto init
	}

	LoadMapSizes()
	szs, ok = WalletAddrsCount[common.AllBalMinVal()]
	if ok {
		//fmt.Println("Have map sizes for MinBal", common.AllBalMinVal(), ":", szs[0], szs[1], szs[2], szs[3])
	} else {
		//fmt.Println("No map sizes for MinBal", common.AllBalMinVal())
		szs = [4]int{10e6, 3e6, 10e3, 1e3} // defaults
	}

init:
	AllBalancesP2KH = make(map[[20]byte]*OneAllAddrBal, szs[0])
	AllBalancesP2SH = make(map[[20]byte]*OneAllAddrBal, szs[1])
	AllBalancesP2WKH = make(map[[20]byte]*OneAllAddrBal, szs[2])
	AllBalancesP2WSH = make(map[[32]byte]*OneAllAddrBal, szs[3])
}

func LoadBalance() {
	if common.GetBool(&common.WalletON) {
		//fmt.Println("wallet.LoadBalance() ignore: ", common.GetBool(&common.WalletON))
		return
	}

	var aborted bool

	common.SetUint32(&common.WalletProgress, 1)
	common.ApplyBalMinVal()

	InitMaps(false)

	common.BlockChain.Unspent.RWMutex.RLock()
	defer common.BlockChain.Unspent.RWMutex.RUnlock()

	cnt_dwn_from := (len(common.BlockChain.Unspent.HashMap) + 999) / 1000
	cnt_dwn := cnt_dwn_from
	perc := uint32(1)

	for k, v := range common.BlockChain.Unspent.HashMap {
		NewUTXO(utxo.NewUtxoRecStatic(k, utxo.Slice(v)))
		if cnt_dwn == 0 {
			perc++
			common.SetUint32(&common.WalletProgress, perc)
			cnt_dwn = cnt_dwn_from
		} else {
			cnt_dwn--
		}
		if FetchingBalanceTick != nil && FetchingBalanceTick() {
			aborted = true
			break
		}
	}
	if aborted {
		InitMaps(true)
	} else {
		common.BlockChain.Unspent.CB.NotifyTxAdd = TxNotifyAdd
		common.BlockChain.Unspent.CB.NotifyTxDel = TxNotifyDel
		common.SetBool(&common.WalletON, true)
	}
	common.SetUint32(&common.WalletProgress, 0)
}

func Disable() {
	if !common.GetBool(&common.WalletON) {
		//fmt.Println("wallet.Disable() ignore: ", common.GetBool(&common.WalletON))
		return
	}
	UpdateMapSizes()
	common.BlockChain.Unspent.CB.NotifyTxAdd = nil
	common.BlockChain.Unspent.CB.NotifyTxDel = nil
	common.SetBool(&common.WalletON, false)
	InitMaps(true)
}

const (
	MAPSIZ_FILE_NAME = "mapsize.gob"
)

var (
	WalletAddrsCount map[uint64][4]int = make(map[uint64][4]int) //index:MinValue, [0]-P2KH, [1]-P2SH, [2]-P2WSH, [3]-P2WKH
)

func UpdateMapSizes() {
	WalletAddrsCount[common.AllBalMinVal()] = [4]int{len(AllBalancesP2KH),
		len(AllBalancesP2SH), len(AllBalancesP2WKH), len(AllBalancesP2WSH)}

	buf := new(bytes.Buffer)
	gob.NewEncoder(buf).Encode(WalletAddrsCount)
	ioutil.WriteFile(common.GocoinHomeDir+MAPSIZ_FILE_NAME, buf.Bytes(), 0600)
}

func LoadMapSizes() {
	d, er := ioutil.ReadFile(common.GocoinHomeDir + MAPSIZ_FILE_NAME)
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
