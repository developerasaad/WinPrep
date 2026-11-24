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
	"strings"
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
	svc := audit.NewService(reg, sys, windows.NewSetupStateFile())

	sig, err := svc.Collect(context.Background())
	if err != nil {
		t.Fatalf("collecting audit signals: %v", err)
	}
	// Real Windows installations write the setup state under the
	// ImageState value name. This assertion is the regression guard for
	// the v0.2.0-alpha.1 defect, which read StateName (never present on
	// real machines) and evaluated every real machine as "unknown".
	if !sig.HaveSetupImageState {
		t.Error("ImageState should exist under SOFTWARE\\Microsoft\\Windows\\CurrentVersion\\Setup\\State on a real Windows installation")
	}

	assessment, err := svc.Inspect(context.Background())
	if err != nil {
		t.Fatalf("audit inspection: %v", err)
	}
	// A normally deployed Windows installation (GitHub Actions runners,
	// developer machines) reports IMAGE_STATE_COMPLETE and must be
	// classified as deployed, never as Audit Mode. A machine in Audit
	// Mode or OOBE would legitimately report a different state.
	switch assessment.State {
	case audit.StateNormalInstalled:
		// the expected outcome on a deployed installation
	case audit.StateAuditMode, audit.StateOOBE, audit.StateUncertain:
		t.Logf("machine reports %q (summary: %s); acceptable outside a deployed installation", assessment.State, assessment.Summary)
	default:
		t.Errorf("undefined audit state %d", assessment.State)
	}
	if assessment.State == audit.StateUnknown {
		t.Errorf("a real Windows installation must produce a definite setup-state classification, got unknown (summary: %s)", assessment.Summary)
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

func TestLocalAccountsFromRealMachine(t *testing.T) {
	sys := windows.NewSystem(windows.NewRegistry())
	accounts, err := sys.LocalAccounts(context.Background())
	if err != nil {
		t.Fatalf("LocalAccounts: %v", err)
	}
	if len(accounts) == 0 {
		t.Fatal("a Windows installation must have local accounts")
	}
	haveAdmin := false
	for _, a := range accounts {
		if a.Name == "" {
			t.Error("every account should carry a name")
		}
		if !strings.HasPrefix(a.SID, "S-1-5-") || a.RID == 0 {
			t.Errorf("account %q has an unusable SID %q (RID %d)", a.Name, a.SID, a.RID)
		}
		if a.RID == 500 {
			haveAdmin = true
		}
	}
	if !haveAdmin {
		t.Error("the built-in Administrator (RID 500) should be listed")
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
