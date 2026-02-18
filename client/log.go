package main

import (
	"io"
	"os"
)

func setupLogging(path string) (cleanup func(), err error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, err
	}

	origStdout := os.Stdout
	origStderr := os.Stderr

	rOut, wOut, err := os.Pipe()
	if err != nil {
		f.Close()
		return nil, err
	}
	rErr, wErr, err := os.Pipe()
	if err != nil {
		f.Close()
		return nil, err
	}

	os.Stdout = wOut
	os.Stderr = wErr

	done := make(chan struct{})

	go func() {
		io.Copy(io.MultiWriter(origStdout, f), rOut)
		close(done)
	}()
	go func() {
		io.Copy(io.MultiWriter(origStderr, f), rErr)
	}()

	cleanup = func() {
		wOut.Close()
		wErr.Close()
		<-done
		f.Close()
	}

	return cleanup, nil
}
