//go:build windows

package windows

import (
	"errors"

	"golang.org/x/sys/windows"
)

// InstanceLock implements platform.InstanceLock with a named kernel mutex in
// the Global namespace, so it is visible across sessions and wins even when
// two operators run WinPrep from different consoles. The handle is owned by
// the process and released by the kernel when it exits, so a crashed run
// never leaves a stale lock behind; a run that finishes without resealing
// releases it explicitly so the operator can retry in the same session.
type InstanceLock struct {
	name string
	// handle keeps the mutex alive until Unlock or process exit.
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

// Unlock implements platform.InstanceLock. Releasing a lock this process
// never acquired is a no-op, mirroring the fake used in tests.
func (l *InstanceLock) Unlock() error {
	if !l.held || l.handle == 0 {
		return nil
	}
	if err := windows.ReleaseMutex(l.handle); err != nil {
		return err
	}
	if err := windows.CloseHandle(l.handle); err != nil {
		return err
	}
	l.handle, l.held = 0, false
	return nil
}
