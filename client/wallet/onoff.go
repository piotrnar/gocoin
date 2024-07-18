package wallet

import (
	"bytes"
	"encoding/gob"
	"io/ioutil"
	"log"

	"github.com/piotrnar/gocoin/client/common"
	"github.com/piotrnar/gocoin/lib/utxo"
)

var (
	FetchingBalanceTick func() bool
	OnOff               chan bool = make(chan bool, 1)
)

func InitMaps(empty bool) {
	var szs [5]int
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
		szs = [5]int{25e6, 8e6, 4e6, 300e3, 1e3} // defaults
	}

init:
	AllBalancesP2KH = make(map[[20]byte]*OneAllAddrBal, szs[0])
	AllBalancesP2SH = make(map[[20]byte]*OneAllAddrBal, szs[1])
	AllBalancesP2WKH = make(map[[20]byte]*OneAllAddrBal, szs[2])
	AllBalancesP2WSH = make(map[[32]byte]*OneAllAddrBal, szs[3])
	AllBalancesP2TAP = make(map[[32]byte]*OneAllAddrBal, szs[4])
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

	iter := common.BlockChain.Unspent.LDB.NewIterator(nil, nil)
	_i := 0
	for iter.Next() {
		var k utxo.UtxoKeyType
		key := iter.Key()
		copy(k[:], key[:])
		v := iter.Value()
		NewUTXO(utxo.NewUtxoRecStatic(k, v))
		if FetchingBalanceTick != nil && FetchingBalanceTick() {
			aborted = true
			break
		}
		if aborted {
			break
		}
		common.SetUint32(&common.WalletProgress, 1000*(uint32(_i)+1)/256)
		_i++
	}
	iter.Release()
	if err := iter.Error(); err != nil {
		log.Fatal(err)
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
	WalletAddrsCount map[uint64][5]int = make(map[uint64][5]int) //index:MinValue, [0]-P2KH, [1]-P2SH, [2]-P2WSH, [3]-P2WKH, [4]-P2TAP
)

func UpdateMapSizes() {
	WalletAddrsCount[common.AllBalMinVal()] = [5]int{len(AllBalancesP2KH),
		len(AllBalancesP2SH), len(AllBalancesP2WKH), len(AllBalancesP2WSH),
		len(AllBalancesP2TAP)}

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
