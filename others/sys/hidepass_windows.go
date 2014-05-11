package sys

import (
	"os"
	"fmt"
)

// New method (requires msvcrt.dll):
import (
	"syscall"
)

var (
	msvcrt = syscall.NewLazyDLL("msvcrt.dll")
	_getch = msvcrt.NewProc("_getch")
)

func getch() int {
	res, _, _ := syscall.Syscall(_getch.Addr(), 0, 0, 0, 0)
	return int(res)
}

func enterpassext(b []byte) (n int) {
	for {
		chr := byte(getch())
		if chr==3 {
			// Ctrl+C
			ClearBuffer(b)
			os.Exit(0)
		}
		if chr==13 || chr==10 {
			fmt.Println()
			break // Enter
		}
		if chr=='\b' {
			if n>0 {
				n--
				b[n] = 0
				fmt.Print("\b \b")
			} else {
				fmt.Print("\007")
			}
			continue
		}
		if chr<' ' {
			fmt.Print("\007")
			fmt.Println("\n", chr)
			continue
		}
		if n==len(b) {
			fmt.Print("\007")
			continue
		}
		fmt.Print("*")
		b[n] = chr
		n++
	}
	return
}

func init() {
	er := _getch.Find()
	if er != nil {
		println(er.Error())
		println("WARNING: Characters will be visible during password input.")
		return
	}

	secrespass = enterpassext
}


/*
Old method (requires mingw):

#include <conio.h>
// end the comment here when enablign this method
import "C"

func getch() int {
	return int(C._getch())
}

*/
