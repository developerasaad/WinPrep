//go:build windows

package handover

import (
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/windows"
)

// makeUndeletable makes removing the contents of dir fail for the duration
// of the test. On Windows, a file opened without FILE_SHARE_DELETE cannot
// be unlinked while the handle is open, which is exactly how a running
// browser holds its data files. The returned undo closes the handle.
func makeUndeletable(t *testing.T, dir string) func() {
	t.Helper()
	victim := filepath.Join(dir, "locked-by-open-handle.txt")
	if err := os.WriteFile(victim, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	path, err := windows.UTF16PtrFromString(victim)
	if err != nil {
		t.Fatal(err)
	}
	handle, err := windows.CreateFile(path, windows.GENERIC_READ, 0, nil,
		windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		t.Fatalf("opening %s without sharing: %v", victim, err)
	}
	return func() { _ = windows.CloseHandle(handle) }
}
