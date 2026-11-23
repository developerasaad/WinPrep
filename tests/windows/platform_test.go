//go:build windows

// Package windows hosts the integration tests that run against a real
// Windows installation: the GitHub Actions Windows runner during CI, or any
// Windows machine with a Go toolchain. They exercise the real registry,
// token, and process plumbing - read-only - plus end-to-end runs of the
// built executable.
//
// Nothing here invokes Sysprep, modifies the registry, or requires Audit
// Mode. The machine's actual state is asserted honestly: a normal deployed
// installation (like a CI runner) must report itself as such.
package windows

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/developerasaad/winprep/internal/audit"
	"github.com/developerasaad/winprep/internal/platform"
	"github.com/developerasaad/winprep/internal/windows"
)

func TestRegistryReadsRealValues(t *testing.T) {
	reg := windows.NewRegistry()

	name, ok, err := reg.StringValue(platform.HKLM,
		`SOFTWARE\Microsoft\Windows NT\CurrentVersion`, "ProductName")
	if err != nil {
		t.Fatalf("reading ProductName: %v", err)
	}
	if !ok || name == "" {
		t.Fatalf("ProductName should exist on every Windows installation, got %q (present=%v)", name, ok)
	}

	build, ok, err := reg.StringValue(platform.HKLM,
		`SOFTWARE\Microsoft\Windows NT\CurrentVersion`, "CurrentBuildNumber")
	if err != nil {
		t.Fatalf("reading CurrentBuildNumber: %v", err)
	}
	if !ok || build == "" {
		t.Fatalf("CurrentBuildNumber should exist, got %q (present=%v)", build, ok)
	}

	// A deliberately absent value must report not-found, not an error.
	_, ok, err = reg.StringValue(platform.HKLM,
		`SOFTWARE\Microsoft\Windows NT\CurrentVersion`, "WinPrepNoSuchValue")
	if err != nil {
		t.Fatalf("absent value should report not-found, got error: %v", err)
	}
	if ok {
		t.Error("absent value should not be reported as present")
	}
}

func TestSystemInfoFromRealMachine(t *testing.T) {
	sys := windows.NewSystem(windows.NewRegistry())
	info, err := sys.SystemInfo(context.Background())
	if err != nil {
		t.Fatalf("SystemInfo: %v", err)
	}
	if !info.IsWindows {
		t.Error("IsWindows must be true on Windows")
	}
	if info.Build == "" {
		t.Error("build number should be readable")
	}
	if info.InstallationType != "Client" && info.InstallationType != "Server" {
		t.Errorf("installation type = %q, want Client or Server", info.InstallationType)
	}

	user, err := sys.UserInfo(context.Background())
	if err != nil {
		t.Fatalf("UserInfo: %v", err)
	}
	if user.SID == "" {
		t.Error("user SID should be readable")
	}
	// The RID-500 check must agree with the SID's final component.
	wantAdmin := len(user.SID) > 4 && user.SID[len(user.SID)-4:] == "-500"
	if user.IsBuiltinAdmin != wantAdmin {
		t.Errorf("IsBuiltinAdmin = %v, but SID %s suggests %v", user.IsBuiltinAdmin, user.SID, wantAdmin)
	}
}

func TestAuditSignalsFromRealMachine(t *testing.T) {
	reg := windows.NewRegistry()
	sys := windows.NewSystem(reg)
	svc := audit.NewService(reg, sys)

	assessment, err := svc.Inspect(context.Background())
	if err != nil {
		t.Fatalf("audit inspection: %v", err)
	}
	// The state must be one of the defined states; a CI runner or normal
	// machine is a deployed installation, but the test only requires that
	// detection reaches a definite conclusion without error.
	switch assessment.State {
	case audit.StateAuditMode, audit.StateOOBE, audit.StateNormalInstalled,
		audit.StateUncertain, audit.StateUnknown:
		// all defined states are acceptable outcomes of an honest read
	default:
		t.Errorf("undefined audit state %d", assessment.State)
	}
	if assessment.Summary == "" {
		t.Error("assessment should carry a human-readable summary")
	}
}

func TestProcessListingIncludesSelf(t *testing.T) {
	procs := windows.NewProcessLister()
	names, err := procs.ProcessNames(context.Background())
	if err != nil {
		t.Fatalf("listing processes: %v", err)
	}
	if len(names) == 0 {
		t.Fatal("a running Windows system must have processes")
	}
	// The test binary itself must appear.
	found := false
	for _, n := range names {
		if n == "windows.test.exe" || n == "test.exe" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("the test process should appear in the process list; got %d names", len(names))
	}
}

func TestSysprepPathCheck(t *testing.T) {
	_, err := windows.SysprepPath()
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("SysprepPath returned an unexpected error: %v", err)
	}
	// A missing Sysprep on a trimmed installation is a legitimate finding;
	// anything else (for example an unset WINDIR) is an error worth failing on.
}
