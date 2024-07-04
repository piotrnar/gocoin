package main

import (
	"fmt"


	"bytes"
	"encoding/hex"
	"os"
	"os/exec"
	"strings"
	"github.com/piotrnar/gocoin/remote-wallet/common"

)

// MessageHandler contains all the logic for handling messages defined in the common.Msg package
type MsgHandler struct {
    WalletBinaryPath string
    WalletFolderPath string
}

func parseWalletCommandArgs(cmd string) []string {
    return (strings.Split(cmd, " "))[1:]
}

var Tx2SignFileName = "tx2sign.txt"
var UnspentFileName = "unspent.txt"
var BalanceFolderName = "balance"

func (h *MsgHandler) createNecessaryFiles(payload common.SignTransactionRequestPayload) error {
    // create tx2sign.txt
    tx2signFilePath := fmt.Sprintf("%s/%s", h.WalletFolderPath, Tx2SignFileName)
    err := os.WriteFile(tx2signFilePath, []byte(payload.Tx2Sign), os.ModePerm)
    if err != nil {
        return err
    }
    balanceFolder := fmt.Sprintf("%s/%s", h.WalletFolderPath, BalanceFolderName)
    err = os.MkdirAll(balanceFolder, os.ModePerm)
    if err != nil {
        return err
    }
    // create unspent.txt
    unspentFilePath := fmt.Sprintf("%s/%s", balanceFolder, UnspentFileName)
    err = os.WriteFile(unspentFilePath, []byte(payload.Unspent), os.ModePerm)
    if err != nil {
        return err
    }
    // create the tx file inside the balance folder. there will be multiple files likes this
    txFile := fmt.Sprintf("%s/%s", balanceFolder, payload.BalanceFileName)
    txUnspent,err := hex.DecodeString(payload.BalanceFileContents)
    if err != nil {
        return err
    }
    err = os.WriteFile(txFile, txUnspent, os.ModePerm)
    if err != nil {
        return err
    }
    return nil
}

var SignedTransactionFileName = "signedtx.txt"

func(h *MsgHandler) SignTransaction(payload interface{}) (string, error) {
    p, err := common.SignTxPayloadFromMapInterface(payload.(map[string]interface{}))
    if err != nil {
        return "", fmt.Errorf("Invalid payload for sign transaction request: %e", err)
    }
    // create necessary files
    fmt.Println("Creating necessary files...")
    err = h.createNecessaryFiles(p)
    if err != nil {
        return "",err
    }
    // run the wallet command
    fmt.Println("Parsing the wallet args...")
    args := parseWalletCommandArgs(p.PayCmd)
    // set custom name for generated signed transaction file
    args = append(args, "-txfn="+SignedTransactionFileName)
    cmd := exec.Command(h.WalletBinaryPath, args...)
    // set wallet folder as the directory of execution
    cmd.Dir = h.WalletFolderPath
    // set a buffer as the stdout
    fmt.Printf("Running the command: %s\n", p.PayCmd)
    out := bytes.NewBuffer(make([]byte, 0))
    cmd.Stdout = out
    err = cmd.Run()
    if err != nil {
        return "", err
    }
    signedTxFileName := fmt.Sprintf("%s/%s", h.WalletFolderPath, SignedTransactionFileName)
    rawHex, _ := os.ReadFile(signedTxFileName)
    return string(rawHex[:]), nil
}
