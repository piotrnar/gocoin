// +build !windows

package sys

import (
	"os"
	"fmt"
	"syscall"
	"os/signal"
)

var wsta syscall.WaitStatus = 0


func enterpassext(b []byte) (n int) {
	si := make(chan os.Signal, 10)
	br := make(chan bool)
	fd := []uintptr{os.Stdout.Fd()}

	signal.Notify(si, syscall.SIGHUP, syscall.SIGINT, syscall.SIGKILL, syscall.SIGQUIT, syscall.SIGTERM)
	go sighndl(fd, si, br)

	pid, er := syscall.ForkExec("/bin/stty", []string{"stty", "-echo"}, &syscall.ProcAttr{Dir: "", Files: fd})
	if er == nil {
		syscall.Wait4(pid, &wsta, 0, nil)
		n = getline(b)
		close(br)
		echo(fd)
		fmt.Println()
	} else {
		n = getline(b)
	}

	return
}


func echo(fd []uintptr) {
	pid, e := syscall.ForkExec("/bin/stty", []string{"stty", "echo"}, &syscall.ProcAttr{Dir: "", Files: fd})
	if e == nil {
		syscall.Wait4(pid, &wsta, 0, nil)
	}
}


func sighndl(fd []uintptr, signal chan os.Signal, br chan bool) {
	select {
		case <-signal:
			echo(fd)
			os.Exit(-1)
		case <-br:
	}
}

func init() {
	secrespass = enterpassext
}
