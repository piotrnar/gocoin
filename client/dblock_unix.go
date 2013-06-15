// +build darwin freebsd linux netbsd openbsd

package main

import (
	"os"
	"syscall"
)

func LockDatabaseDir() {
	var e error
	DbLockFileName = GocoinHomeDir+".lock"
	DbLockFileHndl, e = os.OpenFile(DbLockFileName, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0660)
	if e != nil {
		println(e.Error())
		println("Could not lock the databse folder for writing. Another instance might be running.")
		println("Make sure you can delete and recreate file:", DbLockFileName)
		os.Exit(1)
	}
	syscall.Flock(int(DbLockFileHndl.Fd()), syscall.LOCK_EX)
}