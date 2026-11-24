// Package apptest assembles a fully wired App on top of the platform fakes
// for CLI and TUI tests. It exists so those presentation-layer tests exercise
// the real service assembly (app.Assemble) rather than a parallel one.
package apptest

import (
	"context"
	"os"
	"path/filepath"

	"github.com/developerasaad/winprep/internal/app"
	"github.com/developerasaad/winprep/internal/artifacts"
	"github.com/developerasaad/winprep/internal/platform"
	"github.com/developerasaad/winprep/internal/platform/fake"
)

// FakeSysprep is a handover.SysprepRunner for tests. It records the reseal
// call and returns the configured result.
type FakeSysprep struct {
	Path     string
	PathErr  error
	ExitCode int
	Err      error
	Called   bool
}

// SysprepPath implements handover.SysprepRunner.
func (s *FakeSysprep) SysprepPath() (string, error) {
	if s.PathErr != nil {
		return "", s.PathErr
	}
	if s.Path == "" {
		return `C:\Windows\System32\Sysprep\Sysprep.exe`, nil
	}
	return s.Path, nil
}

// RunReseal implements handover.SysprepRunner.
func (s *FakeSysprep) RunReseal(context.Context) (int, error) {
	s.Called = true
	return s.ExitCode, s.Err
}

// Fixture is a fully wired App over fakes, plus handles to the fakes so
// tests can inspect and reconfigure them.
type Fixture struct {
	App     *app.App
	Sys     *fake.System
	Reg     *fake.Registry
	Run     *fake.Runner
	Procs   *fake.ProcessLister
	Sysprep *FakeSysprep
	Store   *artifacts.Store
	// SysprepLogs feeds the idle-versus-active distinction.
	SysprepLogs *fake.SysprepMonitor
	// Lock simulates the machine-wide operation mutex.
	Lock *fake.InstanceLock
	// DemoProfile is the built-in Administrator's profile root: the
	// demonstration environment the cleanup plan works on.
	DemoProfile string
}

// New returns a fixture whose machine is a plausible Audit Mode laptop:
// Windows 11 Pro, elevated built-in Administrator, all checks green.
func New(dataDir string) *Fixture {
	// The profiles root must be a controlled empty directory, never the real
	// C:\Users. The handover inspection globs it on the live filesystem, and a
	// real Windows machine always has extra folders there (the GitHub runner,
	// for instance, carries a runneradmin profile), which would flip the
	// preflight verdict from Ready to "requires attention" and make the
	// presentation tests fail only on Windows.
	// The profiles root sits beside (not inside) the data directory so the
	// cleanup plan's two scopes stay disjoint, exactly as C:\Users and
	// C:\ProgramData\WinPrep are on a real machine. It must also be a
	// controlled directory, never the real C:\Users: the handover inspection
	// globs it on the live filesystem, and a real Windows machine always has
	// extra folders there (the GitHub runner, for instance, carries a
	// runneradmin profile), which would flip the preflight verdict and make
	// the presentation tests fail only on Windows.
	usersDir := filepath.Join(filepath.Dir(dataDir), "profiles-root")
	if err := os.MkdirAll(usersDir, 0o755); err != nil {
		// dataDir comes from t.TempDir(); failing here means the test
		// environment is broken, not the code under test.
		panic("apptest: creating the profiles root: " + err.Error())
	}

	// The demo profile is the built-in Administrator's profile under the
	// profiles root, registered by SID like a real machine. Downloads and the
	// local Temp folder exist but start empty, so a default fixture produces
	// an empty but well-formed cleanup plan.
	demoProfile := filepath.Join(usersDir, "Administrator")
	for _, d := range []string{
		filepath.Join(demoProfile, "Downloads"),
		filepath.Join(demoProfile, "AppData", "Local", "Temp"),
	} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			panic("apptest: creating the demo profile: " + err.Error())
		}
	}

	reg := fake.NewRegistry().
		SetString(platform.HKLM, `SOFTWARE\Microsoft\Windows\CurrentVersion\Setup\State`, "StateName", "IMAGE_STATE_UNDEPLOYABLE").
		SetString(platform.HKLM, `SYSTEM\Setup`, "CmdLine", "audit.exe").
		SetDWORD(platform.HKLM, `SYSTEM\Setup`, "SystemSetupInProgress", 1).
		SetString(platform.HKLM, `SOFTWARE\Microsoft\Windows NT\CurrentVersion\ProfileList\`+adminSID, "ProfileImagePath", demoProfile).
		SetString(platform.HKU, adminSID+`\SOFTWARE\Microsoft\Windows\CurrentVersion\Explorer\User Shell Folders`,
			"{374DE290-123F-4565-9164-39C4925E467B}", filepath.Join(demoProfile, "Downloads"))

	sys := &fake.System{
		Sys: platform.SystemInfo{
			OSName: "Windows 11 Pro", Build: "10.0.22631.4317", DisplayVersion: "23H2",
			Edition: "Pro", Manufacturer: "LENOVO", Model: "20XW",
			CPU: "Intel Core i5", TotalRAMBytes: 17_179_869_184,
			InstallationType: "Client", Architecture: "x64", IsWindows: true,
		},
		Usr: platform.UserInfo{
			Name: `WKS\Administrator`, SID: "S-1-5-21-1-2-3-500",
			IsElevated: true, IsBuiltinAdmin: true,
		},
		Accounts: []platform.LocalAccount{
			{Name: "Administrator", SID: "S-1-5-21-1-2-3-500", RID: 500},
		},
	}

	run := fake.NewRunner()

	f := &Fixture{
		Sys:         sys,
		Reg:         reg,
		Run:         run,
		Procs:       &fake.ProcessLister{},
		Sysprep:     &FakeSysprep{},
		Store:       artifacts.NewStore(dataDir),
		SysprepLogs: &fake.SysprepMonitor{},
		Lock:        &fake.InstanceLock{},
		DemoProfile: demoProfile,
	}
	f.App = app.Assemble(
		sys, reg, run, f.Procs, f.Store,
		f.Sysprep,
		f.SysprepLogs,
		f.Lock,
		usersDir,
	)
	return f
}

// adminSID is the built-in Administrator of the fixture's machine.
const adminSID = "S-1-5-21-1-2-3-500"
