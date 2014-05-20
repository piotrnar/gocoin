// +build !windows

package sys

import (
	"os"
	"syscall"
)

var (
	DbLockFileName string
	DbLockFileHndl *os.File
)

func LockDatabaseDir(GocoinHomeDir string) {
	var e error
	os.MkdirAll(GocoinHomeDir, 0770)
	DbLockFileName = GocoinHomeDir+".lock"
	DbLockFileHndl, e = os.OpenFile(DbLockFileName, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0660)
	if e != nil {
		goto error
	}
	e = syscall.Flock(int(DbLockFileHndl.Fd()), syscall.LOCK_EX)
	if e != nil {
		goto error
	}
	return

error:
	println(e.Error())
	println("Could not lock the databse folder for writing. Another instance might be running.")
	println("If it is not the case, just delete this file:", DbLockFileName)
	os.Exit(1)
}

func UnlockDatabaseDir() {
	DbLockFileHndl.Close()
	os.Remove(DbLockFileName)
}
