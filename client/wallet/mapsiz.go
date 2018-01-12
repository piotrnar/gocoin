package wallet

import (
	"bufio"
	"encoding/gob"
	"github.com/piotrnar/gocoin/client/common"
	"os"
)

const (
	MAPSIZ_FILE_NAME = "mapsize.gob"
)

var (
	WalletAddrsCount map[uint64][4]int = make(map[uint64][4]int) //index:MinValue, [0]-P2KH, [1]-P2SH, [2]-P2WSH, [3]-P2WKH
)

func UpdateMapSizes() {
	WalletAddrsCount[common.AllBalMinVal()] = [4]int{len(AllBalancesP2KH),
		len(AllBalancesP2SH), len(AllBalancesP2WKH), len(AllBalancesP2WSH)}

	f, er := os.Create(common.GocoinHomeDir + MAPSIZ_FILE_NAME)
	if er != nil {
		println("SaveMapSizes:", er.Error())
		return
	}

	buf := bufio.NewWriter(f)
	gob.NewEncoder(buf).Encode(WalletAddrsCount)

	buf.Flush()
	f.Close()

}

func LoadMapSizes() {
	f, er := os.Open(common.GocoinHomeDir + MAPSIZ_FILE_NAME)
	if er != nil {
		println("LoadMapSizes:", er.Error())
		return
	}

	buf := bufio.NewReader(f)

	er = gob.NewDecoder(buf).Decode(&WalletAddrsCount)
	if er != nil {
		println("LoadMapSizes:", er.Error())
	}

	f.Close()
}
