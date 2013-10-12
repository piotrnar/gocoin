package main

/*
If you cannot build a wallet app, just delete this file and retry.
Then characters will be shown on a console when inputing a password.
*/

/*
#include <conio.h>
*/
import "C"
import (
	"os"
	"fmt"
)

func enterpassext() string {
	var pass string
	for {
		chr := byte(C._getch())
		if chr==3 {
			os.Exit(0)
			// Ctrl+C
		}
		if chr==13 {
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
	secrespass = enterpassext
}
