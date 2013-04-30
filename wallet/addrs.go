package main

import (
	"os"
	"fmt"
	"flag"
	"bufio"
	"math/big"
	"crypto/rand"
	"crypto/ecdsa"
	"encoding/hex"
	"crypto/sha256"
	"github.com/piotrnar/gocoin/btc"
	"code.google.com/p/go.crypto/ripemd160"
)

const (
	TestMessage = "Just some test message"
)

var (
	keycnt *uint = flag.Uint("c", 100, "Set maximum number of keys")
	testnet *bool  = flag.Bool("t", true, "Work with testnet addresses")

	verbyte byte

	maxKeyVal *big.Int
)


func getpass() string {
	f, e := os.Open("wallet.sec")
	if e != nil {
		println("Make sure to create wallet.sec file put your wallet's secret/password into it")
		println(e.Error())
		os.Exit(1)
	}
	le, _ := f.Seek(0, os.SEEK_END)
	buf := make([]byte, le)
	f.Seek(0, os.SEEK_SET)
	n, e := f.Read(buf[:])
	if e != nil {
		println(e.Error())
		os.Exit(1)
	}
	if int64(n)!=le {
		println("Something is wrong with teh password file")
	}
	return string(buf)
}

func getline() string {
	li, _, _ := bufio.NewReader(os.Stdin).ReadLine()
	return string(li)
}


func sharimp160(data []byte) (res [20]byte) {
	sha := sha256.New()
	rim := ripemd160.New()
	sha.Write(data)
	rim.Write(sha.Sum(nil)[:])
	copy(res[:], rim.Sum(nil))
	return
}


func verifyIfKeyWorks(priv []byte, publ []byte) bool {
	hash := btc.Sha2Sum([]byte(TestMessage))
	
	var key ecdsa.PrivateKey
	key.PublicKey.Curve = btc.S256()
	key.PublicKey.X = new(big.Int).SetBytes(publ[1:33])
	key.PublicKey.Y = new(big.Int).SetBytes(publ[33:65])
	
	key.D = new(big.Int).SetBytes(priv)
	if key.D.Cmp(big.NewInt(0)) == 0 {
		println("pubkey value is zero")
		return false
	}
	
	if key.D.Cmp(maxKeyVal) != -1 {
		println("pubkey value is too big", hex.EncodeToString(publ))
		return false
	}

	r, s, err := ecdsa.Sign(rand.Reader, &key, hash[:])
	if err != nil {
		println(err.Error())
		os.Exit(3)
	}

	ok := ecdsa.Verify(&key.PublicKey, hash[:], r, s)
	if !ok {
		println("verfail", hex.EncodeToString(priv))
		println("pubkey", hex.EncodeToString(publ))
		return false
	} else {
		//println("key ver ok:", hex.EncodeToString(priv))
	}
	//hash = btc.Sha2Sum(hash[:])
	return true
}


func getPubKey(curv *btc.BitCurve, priv_key []byte) (res [65]byte) {
	x, y := curv.ScalarBaseMult(priv_key)
	xd := x.Bytes()
	yd := y.Bytes()

	if len(xd)>32 || len(yd)>32 {
		println("x:", len(xd), "y:", len(yd))
		os.Exit(2)
	}

	res[0] = 4
	copy(res[1+32-len(xd):33], xd)
	copy(res[33+32-len(yd):65], yd)
	return
}


func main() {
	if flag.Lookup("h") != nil {
		flag.PrintDefaults()
		os.Exit(0)
	}
	flag.Parse()

	pass := getpass()
	
	if *testnet {
		verbyte = 0x6f
	} else {
		verbyte = 0
	}

	maxKeyVal, _ = new(big.Int).SetString("FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFEBAAEDCE6AF48A03BBFD25E8CD0364141", 16)

	curv := btc.S256()
	seed_key := btc.Sha2Sum([]byte(pass))
	priv_keys := make([][32]byte, *keycnt)
	publ_keys := make([][65]byte, *keycnt)
	publ_addrs := make([]*btc.BtcAddr, *keycnt)
	//fmt.Println("Generating", *keycnt, "keys...")
	var i uint
	for i < *keycnt {
		seed_key = btc.Sha2Sum(seed_key[:])
		priv_keys[i] = seed_key
		publ_keys[i] = getPubKey(curv, seed_key[:])
		if !verifyIfKeyWorks(priv_keys[i][:], publ_keys[i][:]) {
			println("You dont want to use this password - choose another one")
			os.Exit(1)
		}
		h160 := sharimp160(publ_keys[i][:])
		publ_addrs[i] = btc.NewAddrFromHash160(h160[:], verbyte)
		i++
	}
	//fmt.Println("Keys Generated & verified OK")
	
	for i := range priv_keys {
		fmt.Println(publ_addrs[i].String(), " # addr", i+1)
	}
}
