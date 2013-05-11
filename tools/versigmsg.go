package main

import (
	"os"
	"fmt"
	"bytes"
	"strings"
	"math/big"
	"io/ioutil"
	"crypto/ecdsa"
	"encoding/hex"
	"encoding/base64"
	"github.com/piotrnar/gocoin/btc"
)

const BtcMessageMagic = "Bitcoin Signed Message:\n"

var Curv *btc.BitCurve = btc.S256()

type Point struct {
	x, y *big.Int
}

func (p *Point) String() (s string) {
	s = fmt.Sprintln(" x :", BN_bn2hex(p.x))
	s += fmt.Sprintln(" y :", BN_bn2hex(p.x))
	return
}


func BN_bn2hex(b *big.Int) string {
	return strings.ToUpper(hex.EncodeToString(b.Bytes()))
}


/*
Thanks to jackjack for providing me with this nice solution:
https://bitcointalk.org/index.php?topic=162805.msg2112936#msg2112936
*/
func ECRecoverKey(r, s *big.Int, msg []byte, recid int, check bool) (key *ecdsa.PublicKey) {
    order := Curv.N

	x := new(big.Int).Set(order)
	x.Mul(x, big.NewInt(int64(recid/2)))
	x.Add(x, r)

	// Use btc lib to figure out Y for us:
	var xc [33]byte
	xc[0] = 2
	copy(xc[1:], x.Bytes())
	npk, _ := btc.NewPublicKey(xc[:])

	e := new(big.Int).SetBytes(msg)
	e.Neg(e)
	new(big.Int).DivMod(e, order, e)

	inv_r := new(big.Int).ModInverse(r, order)

	RSx, RSy := Curv.ScalarMult(npk.X, npk.Y, s.Bytes())
	Gex, Gey := Curv.ScalarMult(Curv.Gx, Curv.Gy, e.Bytes())
	_x, _y := Curv.Add(RSx, RSy, Gex, Gey)
	Qx, Qy := Curv.ScalarMult(_x, _y, inv_r.Bytes())

	key = new(ecdsa.PublicKey)
	key.Curve = Curv
	key.X = Qx
	key.Y = Qy

	return
}


func main() {
	if len(os.Args) < 3 {
		fmt.Println("Specify at least two parameters:")
		fmt.Println(" 1) The base58 endoided bitcoin addres, that the sugnature shall be checked agains")
		fmt.Println(" 2) The base64 encoded signature for the message...")
		fmt.Println("If you specify a 3rd parameter - this will be assumed to be the message you want to verify")
		fmt.Println("If you do not specify a 3rd parameter - the message will be read from stdin")
		return
	}
	ad, er := btc.NewAddrFromString(os.Args[1])
	if er != nil {
		println("Address:", er.Error())
		return
	}
	//fmt.Println("Public address:", ad.String())

	//b64 := base64.NewEncoding(base64.encodeStd)
	sig, er := base64.StdEncoding.DecodeString(os.Args[2])
	if er != nil {
		println("Signature:", er.Error())
		return
	}
	//fmt.Println("Signature:", hex.EncodeToString(sig))
	if len(sig)!=65 {
		println("Bad signature length", len(sig))
		return
	}

	var msg []byte
	if len(os.Args) < 4 {
		msg, _ = ioutil.ReadAll(os.Stdin)
	} else {
		msg = []byte(os.Args[3])
	}

	b := new(bytes.Buffer)
	btc.WriteVlen(b, uint32(len(BtcMessageMagic)))
	b.Write([]byte(BtcMessageMagic))
	btc.WriteVlen(b, uint32(len(msg)))
	//b.WriteByte(byte())
	b.Write(msg)

	hash := btc.Sha2Sum(b.Bytes())

	nv := sig[0]
	compressed := false
	if nv >= 31 {
		//println("compressed key")
		nv -= 4
		compressed = true
	}

	r := new(big.Int).SetBytes(sig[1:33])
	s := new(big.Int).SetBytes(sig[33:65])
	pub := ECRecoverKey(r, s, hash[:], int(nv-27), false)
	if pub != nil {
		var raw []byte
		if compressed {
			raw = make([]byte, 33)
			raw[0] = byte(2+pub.Y.Bit(0))
			x := pub.X.Bytes()
			copy(raw[1+32-len(x):], x)
		} else {
			raw = make([]byte, 65)
			raw[0] = 4
			x := pub.X.Bytes()
			y := pub.Y.Bytes()
			copy(raw[1+32-len(x):], x)
			copy(raw[1+64-len(y):], y)
		}
		sa := btc.NewAddrFromPubkey(raw, ad.Version)
		ok := ecdsa.Verify(pub, hash[:], r, s)
		if ok {
			if ad.Hash160!=sa.Hash160 {
				fmt.Println("BAD signature for", ad.String())
				os.Exit(1)
			} else {
				fmt.Println("Good signature for", sa.String(), len(msg))
			}
		} else {
			println("BAD signature")
			os.Exit(1)
		}
	} else {
		println("BAD, BAD, BAD signature")
		os.Exit(1)
	}
}
