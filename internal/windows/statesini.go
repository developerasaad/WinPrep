//go:build windows

package windows

import (
	"context"
	"errors"
	"os"
	"path/filepath"
)

// StateIniPath returns %WINDIR%\Setup\State\State.ini, the file Microsoft
// documents alongside the Setup\State registry values in "Windows Setup
// States". The path is resolved from WINDIR the same way SysprepPath is, so
// it follows the running installation rather than a hard-coded drive letter.
func StateIniPath() (string, error) {
	windir := os.Getenv("WINDIR")
	if windir == "" {
		return "", errors.New("WINDIR is not set")
	}
	return filepath.Join(windir, "Setup", "State", "State.ini"), nil
}

// SetupStateFile implements platform.SetupStateFile by reading State.ini
// directly. A missing file is reported as absent rather than an error, which
// matches how the registry treats missing values.
type SetupStateFile struct{}

// NewSetupStateFile returns the Windows State.ini reader.
func NewSetupStateFile() *SetupStateFile { return &SetupStateFile{} }

// StateIniContent implements platform.SetupStateFile.
func (f *SetupStateFile) StateIniContent(context.Context) (string, bool, error) {
	p, err := StateIniPath()
	if err != nil {
		return "", false, err
	}
	data, err := os.ReadFile(p)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", false, nil
		}
		return "", false, err
	}
	return string(data), true, nil
}
