// Package apptest assembles a fully wired App on top of the platform fakes
// for CLI and TUI tests. It exists so those presentation-layer tests exercise
// the real service assembly (app.Assemble) rather than a parallel one.
package apptest

import (
	"context"

	"github.com/developerasaad/winprep/internal/app"
	"github.com/developerasaad/winprep/internal/artifacts"
	"github.com/developerasaad/winprep/internal/platform"
	"github.com/developerasaad/winprep/internal/platform/fake"
)

// HardwareJSON is a realistic diagnostics query result for a two-disk laptop
// with a degraded battery and one problem device.
const HardwareJSON = `{
  "physicalDisks": [
    {"DeviceId": "0", "FriendlyName": "Samsung PM981", "MediaType": "SSD", "BusType": "NVMe", "HealthStatus": "Healthy", "SizeBytes": 512110190592}
  ],
  "diskDrives": [],
  "windowsDisk": 0,
  "battery": [{"Name": "DELL 1VX1H"}],
  "batteryDesigned": 60000,
  "batteryFullCharge": 45000,
  "batteryCycles": 312,
  "problemDevices": [
    {"Name": "Unknown USB Device", "DeviceId": "USB\\VID_0000", "Code": 43}
  ],
  "activation": [{"LicenseStatus": 1, "Edition": "Windows(R), Pro edition"}]
}`

// AccountsJSON is the local-account query result for a machine with no extra
// accounts (only the built-in Administrator, RID 500).
const AccountsJSON = `{"accounts":[{"Name":"Administrator","SID":"S-1-5-21-1-2-3-500","Disabled":false}]}`

// FakeSysprep is a handover.SysprepRunner for tests. It records the reseal
// call and returns the configured result.
type FakeSysprep struct {
	Path    string
	PathErr error
	Res     platform.CommandResult
	Err     error
	Called  bool
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
func (s *FakeSysprep) RunReseal(context.Context) (platform.CommandResult, error) {
	s.Called = true
	return s.Res, s.Err
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
}

// New returns a fixture whose machine is a plausible Audit Mode laptop:
// Windows 11 Pro, elevated built-in Administrator, all checks green.
func New(dataDir string) *Fixture {
	reg := fake.NewRegistry().
		SetString(platform.HKLM, `SOFTWARE\Microsoft\Windows\CurrentVersion\Setup\State`, "StateName", "IMAGE_STATE_UNDEPLOYABLE").
		SetString(platform.HKLM, `SYSTEM\Setup`, "CmdLine", "audit.exe").
		SetDWORD(platform.HKLM, `SYSTEM\Setup`, "SystemSetupInProgress", 1)

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
	}

	run := fake.NewRunner()
	// The fragments distinguish the two PowerShell queries: only the hardware
	// script mentions Get-PhysicalDisk, and only the account script mentions
	// Win32_UserAccount.
	run.SetScriptResult("powershell.exe", "Get-PhysicalDisk", platform.CommandResult{Stdout: HardwareJSON})
	run.SetScriptResult("powershell.exe", "Win32_UserAccount", platform.CommandResult{Stdout: AccountsJSON})
	run.SetResult("powercfg.exe", platform.CommandResult{})

	f := &Fixture{
		Sys:     sys,
		Reg:     reg,
		Run:     run,
		Procs:   &fake.ProcessLister{},
		Sysprep: &FakeSysprep{},
		Store:   artifacts.NewStore(dataDir),
	}
	f.App = app.Assemble(
		sys, reg, run, f.Procs, f.Store,
		"powershell.exe", "powercfg.exe",
		`C:\Windows\System32\Sysprep\Sysprep.exe`,
		f.Sysprep,
		`C:\Users`,
	)
	return f
}
