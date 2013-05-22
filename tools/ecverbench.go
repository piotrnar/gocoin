package main

import (
	"os"
	"net"
	"time"
	"math/big"
	"encoding/hex"
	"github.com/piotrnar/gocoin/btc"
)


func Verify2(kd []byte, sd []byte, hash []byte) bool {
	pub, _ := btc.NewPublicKey(kd)
	sig, _ := btc.NewSignature(sd)

	// See [NSA] 3.4.2
	c := pub.Curve
	N := c.Params().N

	e := new(big.Int).SetBytes(hash)
	w := new(big.Int).ModInverse(sig.S, N)

	u1 := e.Mul(e, w)
	u1.Mod(u1, N)
	u2 := w.Mul(sig.R, w)
	u2.Mod(u2, N)

	x1, y1 := c.ScalarBaseMult(u1.Bytes())
	x2, y2 := c.ScalarMult(pub.X, pub.Y, u2.Bytes())
	x, y := c.Add(x1, y1, x2, y2)
	if x.Sign() == 0 && y.Sign() == 0 {
		return false
	}
	x.Mod(x, N)
	return x.Cmp(sig.R) == 0
}


func Verify(kd []byte, sd []byte, hash []byte) bool {
	var res [1]byte
	conn, e := net.DialTCP("tcp4", nil, &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: int(16667)})
	if e != nil {
		println(e.Error())
		return false
	}
	conn.Write([]byte{1})
	conn.Write([]byte{byte(len(kd))})
	conn.Write(kd)
	conn.Write([]byte{byte(len(sd))})
	conn.Write(sd)
	conn.Write(hash[0:32])
	conn.Read(res[:])
	conn.Close()
	return res[0]!=0
}


var CNT int = 500
const THREADS = 8

func main() {
	if len(os.Args)>1 {
		btc.EcdsaServer = &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: int(16667)}
		CNT *= 20
	}

	key, _ := hex.DecodeString("040eaebcd1df2df853d66ce0e1b0fda07f67d1cabefde98514aad795b86a6ea66dbeb26b67d7a00e2447baeccc8a4cef7cd3cad67376ac1c5785aeebb4f6441c16")
	sig, _ := hex.DecodeString("3045022100fe00e013c244062847045ae7eb73b03fca583e9aa5dbd030a8fd1c6dfcf11b1002207d0d04fed8fa1e93007468d5a9e134b0a7023b6d31db4e50942d43a250f4d07c01")
	msg, _ := hex.DecodeString("3382219555ddbb5b00e0090f469e590ba1eae03c7f28ab937de330aa60294ed6")
	sta := time.Now()
	ch := make(chan bool, THREADS)
	for i:=0; i<THREADS; i++ {
		ch <- true
	}
	var ok bool
	for i:=0; i<CNT; i++ {
		ok = <- ch
		if !ok {
			println("Verify error")
			return
		}
		go func(k, s, h []byte) {
			ch <- btc.EcdsaVerify(k, s, h)
		}(key, sig, msg)
	}
	for i:=0; i<THREADS; i++ {
		ok = <- ch
		if !ok {
			println("Verify error")
			return
		}
	}
	if len(ch)!=0 {
		panic("channel not empty")
	}
	sto := time.Now()
	println((sto.UnixNano()-sta.UnixNano())/int64(CNT*1000), "us per ECDSA_Verify")
}
