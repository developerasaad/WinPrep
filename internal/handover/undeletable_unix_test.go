//go:build !windows

package handover

import (
	"os"
	"testing"
)

// makeUndeletable makes removing the contents of dir fail for the duration
// of the test. On Unix, removing the parent's write permission blocks
// unlinking its children. The returned undo restores the permission.
func makeUndeletable(t *testing.T, dir string) func() {
	t.Helper()
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	return func() { _ = os.Chmod(dir, 0o700) }
}
