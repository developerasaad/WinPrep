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

// PowerShellPath returns the full path of Windows PowerShell 5.1, which ships
// with every supported Windows 10/11 installation. WinPrep does not depend on
// PowerShell 7.
func PowerShellPath() string {
	windir := os.Getenv("WINDIR")
	if windir == "" {
		return "powershell.exe"
	}
	return filepath.Join(windir, "System32", "WindowsPowerShell", "v1.0", "powershell.exe")
}

// PowerCfgPath returns the full path of powercfg.exe, used for battery
// reports.
func PowerCfgPath() string {
	windir := os.Getenv("WINDIR")
	if windir == "" {
		return "powercfg.exe"
	}
	return filepath.Join(windir, "System32", "powercfg.exe")
}

// DataDir returns the directory where WinPrep keeps its managed-artifact
// manifest (%ProgramData%\WinPrep). It creates the directory if needed; that
// is one of the very few things WinPrep writes outside its own process.
func DataDir() (string, error) {
	base := os.Getenv("ProgramData")
	if base == "" {
		base = `C:\ProgramData`
	}
	dir := filepath.Join(base, "WinPrep")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	return dir, nil
}

// UsersDir returns the profiles root (typically C:\Users).
func UsersDir() string {
	return filepath.Join(os.Getenv("SystemDrive"), "Users")
}
