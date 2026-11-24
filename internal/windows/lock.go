//go:build windows

package windows

import (
	"errors"

	"golang.org/x/sys/windows"
)

// InstanceLock implements platform.InstanceLock with a named kernel mutex in
// the Global namespace, so it is visible across sessions and wins even when
// two operators run WinPrep from different consoles. The handle is owned by
// the process and released by the kernel when it exits, which is the behavior
// the operation controller needs: a crashed run never leaves a stale lock
// behind.
type InstanceLock struct {
	name string
	// handle keeps the mutex alive for the process lifetime once acquired.
	handle windows.Handle
	held   bool
}

// NewInstanceLock returns the Windows operation lock.
func NewInstanceLock() *InstanceLock {
	return &InstanceLock{name: `Global\WinPrep-Clean-Finish`}
}

// TryLock implements platform.InstanceLock.
func (l *InstanceLock) TryLock() (bool, error) {
	if l.held {
		return false, nil
	}
	namePtr, err := windows.UTF16PtrFromString(l.name)
	if err != nil {
		return false, err
	}
	handle, err := windows.CreateMutex(nil, false, namePtr)
	switch {
	case err == nil:
		l.handle, l.held = handle, true
		return true, nil
	case errors.Is(err, windows.ERROR_ALREADY_EXISTS):
		// The mutex exists, so another WinPrep process holds it. The handle
		// we received is still valid and must be closed.
		_ = windows.CloseHandle(handle)
		return false, nil
	default:
		return false, err
	}
}
