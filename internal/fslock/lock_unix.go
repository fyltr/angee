//go:build darwin || linux

package fslock

import (
	"errors"
	"os"
	"syscall"
)

func tryLockFile(file *os.File) error {
	return flock(file, syscall.LOCK_EX|syscall.LOCK_NB)
}

func unlockFile(file *os.File) error {
	return flock(file, syscall.LOCK_UN)
}

func isLockBusy(err error) bool {
	return errors.Is(err, syscall.EWOULDBLOCK)
}

func flock(file *os.File, how int) error {
	_, _, errno := syscall.Syscall(syscall.SYS_FLOCK, file.Fd(), uintptr(how), 0)
	if errno != 0 {
		return errno
	}
	return nil
}
