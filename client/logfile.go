package main

import (
	"fmt"
	"os"
	"time"

	"github.com/piotrnar/gocoin"
	"github.com/piotrnar/gocoin/client/usif/vcon"
)

func setupLogging(path string) (cleanup func(), err error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, err
	}

	fmt.Fprintln(f, time.Now().Format("2006-01-03 15:04:05"), "starting Gocoin client version", gocoin.Version, " PID", os.Getpid())

	vcon.AddWriter(f)

	cleanup = func() {
		vcon.Close()
		vcon.DelWriter(f)
		f.Close()
	}

	return cleanup, nil
}
