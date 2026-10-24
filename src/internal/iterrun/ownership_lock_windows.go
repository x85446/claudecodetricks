//go:build windows

package iterrun

import (
	"fmt"
	"os"
	"syscall"
	"unsafe"
)

var (
	ownerKernel32         = syscall.NewLazyDLL("ownerKernel32.dll")
	ownerProcLockFileEx   = ownerKernel32.NewProc("LockFileEx")
	ownerProcUnlockFileEx = ownerKernel32.NewProc("UnlockFileEx")
)

// ownerLockFile blocks until it holds an exclusive lock on the first byte of f.
// Windows releases it when the handle closes or the process exits.
func ownerLockFile(f *os.File) error {
	const lockfileExclusiveLock = 0x2
	var ol syscall.Overlapped
	ok, _, err := ownerProcLockFileEx.Call(f.Fd(), lockfileExclusiveLock, 0, 1, 0, uintptr(unsafe.Pointer(&ol)))
	if ok == 0 {
		return err
	}
	return nil
}

func ownerUnlockFile(f *os.File) {
	var ol syscall.Overlapped
	_, _, _ = ownerProcUnlockFileEx.Call(f.Fd(), 0, 1, 0, uintptr(unsafe.Pointer(&ol)))
}

func ownerProcessIdentity(pid int) string {
	const query = 0x1000
	h, e := syscall.OpenProcess(query, false, uint32(pid))
	if e != nil {
		return ""
	}
	defer syscall.CloseHandle(h)
	var created, exited, kernel, user syscall.Filetime
	if e = syscall.GetProcessTimes(h, &created, &exited, &kernel, &user); e != nil {
		return ""
	}
	return fmt.Sprintf("%d:%d", created.HighDateTime, created.LowDateTime)
}
