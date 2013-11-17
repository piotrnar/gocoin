package utils

import (
	"os"
)


func BitcoinHome() (res string) {
	res = os.Getenv("APPDATA")
	if res!="" {
		res += "\\Bitcoin\\"
		return
	}
	res = os.Getenv("HOME")
	if res!="" {
		res += "/.bitcoin/"
	}
	return
}
