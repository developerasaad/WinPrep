//go:build windows

package app

import (
	"context"

	"github.com/developerasaad/winprep/internal/artifacts"
	"github.com/developerasaad/winprep/internal/windows"
)

// New assembles the application with the real Windows implementations.
func New() *App {
	reg := windows.NewRegistry()
	sys := windows.NewSystem(reg)
	run := windows.NewRunner()
	procs := windows.NewProcessLister()

	dataDir, err := windows.DataDir()
	if err != nil {
		// The artifact store falls back to the standard path rather than
		// taking the whole tool down; any later failure to use it is
		// surfaced by the handover preflight.
		dataDir = `C:\ProgramData\WinPrep`
	}

	return Assemble(
		sys, reg, run, procs,
		artifacts.NewStore(dataDir),
		&windowsSysprep{runner: run},
		windows.UsersDir(),
	)
}

// windowsSysprep implements handover.SysprepRunner with the Microsoft
// documented reseal command: Sysprep.exe /oobe /shutdown. No /generalize
// (this is a same-device handover, not an image build) and no /quiet
// (hiding Sysprep's own diagnostics would make failures harder to see).
type windowsSysprep struct {
	runner *windows.CommandRunner
}

func (s *windowsSysprep) SysprepPath() (string, error) {
	return windows.SysprepPath()
}

func (s *windowsSysprep) RunReseal(ctx context.Context) (int, error) {
	path, err := windows.SysprepPath()
	if err != nil {
		return 0, err
	}
	return s.runner.RunVisible(ctx, path, "/oobe", "/shutdown")
}
