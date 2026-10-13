//go:build windows

package testmaster

import (
	"os"
	"syscall"
	"unsafe"
)

var procLockFileEx = kernel32.NewProc("LockFileEx")

// lockFile takes an exclusive, non-blocking lock on the first byte of f.
// Windows releases it when the handle closes or the process exits, however it
// exits.
func lockFile(f *os.File) error {
	const lockfileFailImmediately, lockfileExclusiveLock = 0x1, 0x2
	var ol syscall.Overlapped
	ok, _, err := procLockFileEx.Call(f.Fd(), lockfileExclusiveLock|lockfileFailImmediately, 0, 1, 0, uintptr(unsafe.Pointer(&ol)))
	if ok == 0 {
		return err
	}
	return nil
}
