package main

import (
	"fmt"


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
    file, err := os.Create(tx2signFilePath)
    if err != nil {
        return err
    }
    fmt.Println("writing string: ", payload.Tx2Sign)
    _, err = file.WriteString(payload.Tx2Sign)
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
    args = append(args, "-prompt")
    fmt.Println("printing args, ", args)
    cmd := exec.Command(h.WalletBinaryPath, args...)
    // set wallet folder as the directory of execution
    cmd.Dir = h.WalletFolderPath
    // set a buffer as the stdout
    fmt.Printf("Running the command: %s\n", cmd.Path)
    // out := bytes.NewBuffer(make([]byte, 0))
    cmd.Stdout = os.Stdout
    cmd.Stdin = os.Stdin
    err = cmd.Run()
    if err != nil {
        return "", err
    }
    signedTxFileName := fmt.Sprintf("%s/%s", h.WalletFolderPath, SignedTransactionFileName)
    rawHex, _ := os.ReadFile(signedTxFileName)
    return string(rawHex[:]), nil
}
