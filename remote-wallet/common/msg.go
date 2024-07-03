package common

import "fmt"

type MsgType string
const (
	SignTransaction MsgType = "sign_transaction"
    SignedTransactionRawHex MsgType = "signed_tx_rawhex"
    InitiateConnection MsgType = "initiate_connection"
)

type Msg struct {
    Type MsgType `json:"type"`
    Payload interface{} `json:"payload"`
}

// The connection has to be establised from the Remote Wallet Server. Therefore, the Wallet Remote Server will first send a request to the gocoin node sideapp with the information needed to establish a connection with it. No random client will be able to connect with the Remote Wallet Server. 
// For now the payload should be a string. In the future this will change though. 
type InitiateConnectionRequestPayload string


type SignTransactionResponsePayload string

// TODO: Make it so that it supports multiple balance files 
type SignTransactionRequestPayload struct {
    PayCmd string `json:"payCmd"`
    Tx2Sign string `json:"tx2Sign"`
    Unspent string `json:"unspent"`
    BalanceFileName string `json:"balanceFileName"`
    BalanceFileContents string `json:"balanceFileContents"`
}

func SignTxPayloadFromMapInterface(mi map[string]interface{}) (SignTransactionRequestPayload, error) {
    payload := SignTransactionRequestPayload{}
    PayCmd, ok := mi["payCmd"].(string)
    if !ok {
        return payload, fmt.Errorf("Invalid payload data. Could not find 'payCmd' in the map.")
    }
    TxwSign, ok := mi["tx2Sign"].(string)
    if !ok {
        return payload, fmt.Errorf("Invalid payload data. Could not find 'tx2Sign' in the map.")
    }
    Unspent, ok := mi["unspent"].(string)
    if !ok {
        return payload, fmt.Errorf("Invalid payload data. Could not find 'unspent' in the map.")
    }
    BalanceFileName, ok := mi["balanceFileName"].(string)
    if !ok {
        return payload, fmt.Errorf("Invalid payload data. Could not find 'unspent' in the map.")
    }
    BalanceFileContents, ok := mi["balanceFileContents"].(string)
    if !ok {
        return payload, fmt.Errorf("Invalid payload data. Could not find 'unspent' in the map.")
    }
    payload.PayCmd = PayCmd
    payload.Tx2Sign = TxwSign
    payload.Unspent = Unspent
    payload.BalanceFileName = BalanceFileName
    payload.BalanceFileContents = BalanceFileContents

    return payload, nil
}
