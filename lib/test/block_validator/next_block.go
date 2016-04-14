package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"github.com/piotrnar/gocoin/lib/btc"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"strconv"
)

func main() {
	if len(os.Args) != 6 {
		fmt.Println("Expected 5 command line args: bool bool hexdata uint string")
		os.Exit(1)
	}

	bl, _ := ioutil.ReadFile("dupa.bin")
	if len(bl) < 8 {
		fmt.Println("Incorrect dupa.bin")
		os.Exit(1)
	}

	if binary.BigEndian.Uint32(bl[0:4]) != 0xfabfb5da {
		fmt.Println("Incorrect magic inside dupa.bin")
		os.Exit(1)
	}

	if binary.LittleEndian.Uint32(bl[4:8]) != uint32(len(bl)-8) {
		fmt.Println("Incorrect length inside dupa.bin")
		os.Exit(1)
	}

	arg1 := os.Args[1] == "true"
	arg2 := os.Args[2] == "true"
	arg3 := btc.NewUint256FromString(os.Args[3])
	arg4, _ := strconv.ParseUint(os.Args[4], 10, 32)
	arg5 := url.QueryEscape(os.Args[5])

	u := "http://127.0.0.1:18444/"
	u += fmt.Sprint("?connect=", arg1)
	u += fmt.Sprint("&exception=", arg2)
	u += "&newtop=" + arg3.String()
	u += fmt.Sprint("&newheight=", arg4)
	u += "&blockid=" + arg5

	httpreq, _ := http.NewRequest("POST", u, bytes.NewReader(bl[8:]))
	httpreq.Header.Add("Content-Type", "application/octet-stream")
	httpresp, er := http.DefaultClient.Do(httpreq)
	if er != nil {
		fmt.Println(httpresp, er.Error())
		os.Exit(1)
	}

	txt, _ := ioutil.ReadAll(httpresp.Body)
	if string(txt)!="ok" {
		fmt.Println(string(txt))
		os.Exit(1)
	}

	os.Exit(0)
}
