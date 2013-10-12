package main

/*
Copy this file one level up and (if the wallet still builds),
the passwords characters will be hidden while entering them.
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
		if chr=='\b' && pass!="" {
			pass = pass[0:len(pass)-1]
			fmt.Print("\b \b")
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
