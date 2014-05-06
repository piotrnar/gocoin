package main

import (
	"os"
	"bufio"
	"strings"
	"github.com/piotrnar/gocoin/btc"
)

// Resolved while parsing "-send" parameter
type oneSendTo struct {
	addr *btc.BtcAddr
	amount uint64
}

// parse the "-send ..." parameter
func parse_spend() {
	outs := strings.Split(*send, ",")

	for i := range outs {
		tmp := strings.Split(strings.Trim(outs[i], " "), "=")
		if len(tmp)!=2 {
			println("The otputs must be in a format address1=amount1[,addressN=amountN]")
			os.Exit(1)
		}

		a, e := btc.NewAddrFromString(tmp[0])
		if e != nil {
			println("NewAddrFromString:", e.Error())
			os.Exit(1)
		}

		am, er := btc.StringToSatoshis(tmp[1])
		if er != nil {
			println("Incorrect amount: ", tmp[1], er.Error())
			os.Exit(1)
		}
		if *subfee {
			am -= curFee
		}

		sendTo = append(sendTo, oneSendTo{addr:a, amount:am})
		spendBtc += am
	}
}


func parse_batch() {
	f, e := os.Open(*batch)
	if e == nil {
		defer f.Close()
		td := bufio.NewReader(f)
		var lcnt int
		for {
			li, _, _ := td.ReadLine()
			if li == nil {
				break
			}
			lcnt++
			tmp := strings.SplitN(strings.Trim(string(li), " "), "=", 2)
			if len(tmp)<2 {
				println("Error in the batch file line", lcnt)
				os.Exit(1)
			}
			if tmp[0][0]=='#' {
				continue // Just a comment-line
			}

			a, e := btc.NewAddrFromString(tmp[0])
			if e != nil {
				println("NewAddrFromString:", e.Error())
				os.Exit(1)
			}

			am, e := btc.StringToSatoshis(tmp[1])
			if e != nil {
				println("StringToSatoshis:", e.Error())
				os.Exit(1)
			}

			sendTo = append(sendTo, oneSendTo{addr:a, amount:am})
			spendBtc += am
		}
	} else {
		println(e.Error())
		os.Exit(1)
	}
}


func send_request() bool {
	feeBtc = curFee
	if *send!="" {
		parse_spend()
	}
	if *batch!="" {
		parse_batch()
	}
	return len(sendTo)>0
}
