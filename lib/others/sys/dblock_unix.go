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
	os.MkdirAll(GocoinHomeDir, 0770)
	DbLockFileName = GocoinHomeDir+".lock"
	DbLockFileHndl, _ = os.Open(DbLockFileName)
	if DbLockFileHndl==nil {
		DbLockFileHndl, _ = os.Create(DbLockFileName)
	}
	if DbLockFileHndl==nil {
		goto error
	}

	if e:=syscall.Flock(int(DbLockFileHndl.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); e!=nil {
		goto error
	}
	return

error:
	println("Could not lock the databse folder for writing. Another instance might be running.")
	println("If it is not the case, remove this file:", DbLockFileName)
	os.Exit(1)
}

func UnlockDatabaseDir() {
	syscall.Flock(int(DbLockFileHndl.Fd()), syscall.LOCK_UN)
	DbLockFileHndl.Close()
	os.Remove(DbLockFileName)
}
