package common

import "errors"

func SignTransactionRejectedError() error {
    return errors.New("Your request to sign the transaction has been rejected by the wallet user.")
}
