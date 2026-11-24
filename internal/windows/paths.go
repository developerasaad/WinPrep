//go:build windows

package windows

import (
	"errors"
	"os"
	"path/filepath"
)

// SysprepPath returns the full path of Sysprep.exe. WinPrep always invokes it
// by absolute path so a tampered PATH entry cannot substitute a different
// executable.
func SysprepPath() (string, error) {
	windir := os.Getenv("WINDIR")
	if windir == "" {
		return "", errors.New("WINDIR is not set")
	}
	p := filepath.Join(windir, "System32", "Sysprep", "Sysprep.exe")
	if _, err := os.Stat(p); err != nil {
		return p, err
	}
	return p, nil
}

// DataDir returns the directory where WinPrep keeps its managed-artifact
// manifest (%ProgramData%\WinPrep). The directory is only created when
// something is actually written there (see artifacts.Store.Save), so running
// read-only commands - including clean --dry-run - does not modify the
// system.
func DataDir() (string, error) {
	base := os.Getenv("ProgramData")
	if base == "" {
		base = `C:\ProgramData`
	}
	return filepath.Join(base, "WinPrep"), nil
}

// UsersDir returns the profiles root (typically C:\Users), or an empty
// string when SystemDrive is not set. The handover treats an empty root as
// "orphaned-profile scan unavailable" and reports it, rather than scanning
// an arbitrary relative path.
func UsersDir() string {
	drive := os.Getenv("SystemDrive")
	if drive == "" {
		return ""
	}
	return filepath.Join(drive, "Users")
}
