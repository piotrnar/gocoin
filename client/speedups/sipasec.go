package main

/*
  This is a EC_Verify speedup that is advised for non Windows systems.

  1) Build and install sipa's secp256k1 lib for your system

  2) Copy this file one level up and remove "speedup.go" from there

  3) Rebuild clinet.exe and enjoy sipa's verify lib.
*/

import (
	"github.com/piotrnar/gocoin/client/common"
	"github.com/piotrnar/gocoin/lib/btc"
	"github.com/piotrnar/gocoin/lib/others/cgo/sipasec"
)

func sipa_ec_verify(k, s, h []byte) bool {
	return sipasec.EC_Verify(k, s, h) == 1
}

func schnorr_ec_verify(pkey, sign, msg []byte) bool {
	return sipasec.Schnorr_Verify(pkey, sign, msg) == 1
}

func check_pay_to_contract(m_keydata, base, hash []byte, parity bool) bool {
	return sipasec.CheckPayToContract(m_keydata, base, hash, parity) == 1
}

func init() {
	common.Log.Println("Using libsecp256k1.a for ECVerify, SchnorrVerify & CheckPayToContact")
	btc.EC_Verify = sipa_ec_verify
	btc.Schnorr_Verify = schnorr_ec_verify
	btc.Check_PayToContract = check_pay_to_contract
}
