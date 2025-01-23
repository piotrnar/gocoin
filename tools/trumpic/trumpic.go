package main

import (
	"fmt"
	"image"
	"image/color"
	"image/gif"
	"os"

	"github.com/piotrnar/gocoin/lib/btc"
	"github.com/piotrnar/gocoin/lib/others/utils"
)

var (
	//unicode = []rune{' ', '◕', '◑', '◔'}
	unicode = []rune{' ', '●', '◑', '○'}
	ascii   = []rune{32, 178, 177, 176}
)

func fee2val(fee uint64) int {
	if fee < 1000 {
		return 0
	} else if fee < 10000 {
		return 1
	} else if fee < 20000 {
		return 2
	}
	return 3
}

func main() {
	var bl *btc.Block
	var er error
	var d []byte

	fmt.Println("https://mempool.space/block/" + block_hash + "?audit=false")
	raw_block_file := "blocks/" + block_hash + ".bin"

	if d, er = os.ReadFile(raw_block_file); er == nil {
		if bl, er = btc.NewBlock(d); er == nil {
			bl.BuildTxList()
			fmt.Println("Block loaded from disk")
		}
	}
	if bl == nil {
		println("Block not found on disk in - try to fetch it from web...")
		bl = utils.GetBlockFromWeb(btc.NewUint256FromString(block_hash))
		if bl == nil {
			println("Unable to fetch bitcoin block 879613. Do you have interent connection?")
			return
		}
		os.WriteFile(raw_block_file, bl.Raw, 0600)
	}

	os.Mkdir("out", 0700)
	if f, er := os.Create("out/message.txt"); er == nil {
		for i, tx := range bl.Txs {
			if i != 0 {
				if len(tx.TxOut[1].Pk_script) > 2 && tx.TxOut[1].Pk_script[0] == 0x6a {
					fmt.Fprintln(f, string(tx.TxOut[1].Pk_script[2:]))
				}
			}
		}
		f.Close()
		fmt.Println("Message written to out/message.txt")
	} else {
		println(er.Error())
	}

	img := image.NewPaletted(image.Rect(0, 0, 86, 86), color.Palette{
		color.RGBA{85, 125, 0, 255},
		color.RGBA{187, 125, 17, 255},
		color.RGBA{189, 92, 40, 255},
		color.RGBA{186, 50, 67, 255},
	})
	var ss []string
	var ssu []string
	var s string = " "
	var su string = " "
	var cnt int = 1
	for _, x := range block_fees {
		val := fee2val(x)
		img.SetColorIndex(cnt, 86-len(ss)-1, uint8(val))
		s += string(rune(ascii[val]))
		su += string(rune(unicode[val]))
		cnt++
		if cnt == 86 {
			ss = append(ss, s)
			ssu = append(ssu, su)
			cnt = 0
			s = ""
			su = ""
		}
	}

	if f, er := os.Create("out/picture.ascii"); er == nil {
		for i := range ss {
			fmt.Fprintln(f, ss[len(ss)-i-1])
		}
		fmt.Println("Pictire written to out/picture.ascii")
	} else {
		println(er.Error())
	}

	if f, er := os.Create("out/picture.unicode"); er == nil {
		for i := range ssu {
			fmt.Fprintln(f, ssu[len(ssu)-i-1])
		}
		fmt.Println("Pictire written to out/picture.unicode")
	} else {
		println(er.Error())
	}

	if f, er := os.Create("out/picture.gif"); er == nil {
		gif.Encode(f, img, nil)
		fmt.Println("Pictire written to out/picture.gif")
	} else {
		println(er.Error())
	}
}
