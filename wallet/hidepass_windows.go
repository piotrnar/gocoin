package main

import (
	"os"
	"fmt"
)

// New method (requires msvcrt.dll):
import (
	"syscall"
)

var (
	msvcrt *syscall.LazyDLL
	_getch *syscall.LazyProc
)

func getch() int {
	res, _, _ := syscall.Syscall(_getch.Addr(), 0, 0, 0, 0)
	return int(res)
}

func enterpassext() string {
	var pass string
	for {
		chr := byte(getch())
		if chr==3 {
			os.Exit(0)
			// Ctrl+C
		}
		if chr==13 || chr==10 {
			fmt.Println()
			break // Enter
		}
		if chr=='\b' {
			if len(pass)>0 {
				pass = pass[0:len(pass)-1]
				fmt.Print("\b \b")
			}
			continue
		}
		if chr<' ' {
			fmt.Print("\007")
			fmt.Println("\n", chr)
			continue
		}
		fmt.Print("*")
		pass += string(chr)
	}
	return pass
}

func init() {
	msvcrt = syscall.NewLazyDLL("msvcrt.dll")
	if msvcrt == nil {
		println("Could not open MSVCRT.DLL - characters will be visible during passowrd input.")
		return
	}
	_getch = msvcrt.NewProc("_getch")
	if msvcrt == nil {
		println("Cannot find _getch in MSVCRT.DLL - characters will be visible during passowrd input.")
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
