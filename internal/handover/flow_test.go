package handover

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/developerasaad/winprep/internal/artifacts"
	"github.com/developerasaad/winprep/internal/audit"
	"github.com/developerasaad/winprep/internal/platform"
	"github.com/developerasaad/winprep/internal/platform/fake"
)

// fakeSysprep records whether the reseal was attempted.
type fakeSysprep struct {
	path      string
	pathErr   error
	result    platform.CommandResult
	runErr    error
	ressealed bool
}

func (s *fakeSysprep) SysprepPath() (string, error) { return s.path, s.pathErr }

func (s *fakeSysprep) RunReseal(context.Context) (platform.CommandResult, error) {
	s.ressealed = true
	return s.result, s.runErr
}

// auditReadyRegistry returns a registry that reports a consistent Audit Mode.
func auditReadyRegistry() *fake.Registry {
	return fake.NewRegistry().
		SetString(platform.HKLM, `SOFTWARE\Microsoft\Windows\CurrentVersion\Setup\State`, "StateName", "IMAGE_STATE_UNDEPLOYABLE").
		SetString(platform.HKLM, `SYSTEM\Setup`, "CmdLine", "audit.exe")
}

func newTestFlowWithRegistry(t *testing.T, reg *fake.Registry) (*Flow, *fake.Runner, *fakeSysprep, *artifacts.Store) {
	t.Helper()
	store := artifacts.NewStore(t.TempDir())
	sys := &fake.System{
		Sys: platform.SystemInfo{
			OSName: "Windows 11 Pro", Build: "10.0.22631.4317",
			InstallationType: "Client", Architecture: "x64", IsWindows: true,
		},
		Usr: platform.UserInfo{IsElevated: true, SID: "S-1-5-21-1-2-3-500", IsBuiltinAdmin: true},
	}
	runner := fake.NewRunner()
	runner.SetResult("powershell.exe", platform.CommandResult{Stdout: `{"accounts":[]}`})
	sp := &fakeSysprep{path: `C:\Windows\System32\Sysprep\Sysprep.exe`}
	flow := &Flow{
		Sys:      sys,
		Reg:      reg,
		Run:      runner,
		Procs:    &fake.ProcessLister{Names: []string{"winprep.exe"}},
		Audit:    audit.NewService(reg, sys),
		Store:    store,
		Sysprep:  sp,
		PSPath:   `C:\Windows\System32\WindowsPowerShell\v1.0\powershell.exe`,
		UsersDir: filepath.Join(t.TempDir(), "Users"),
	}
	return flow, runner, sp, store
}

func TestInspectReadyMachine(t *testing.T) {
	flow, _, _, _ := newTestFlowWithRegistry(t, auditReadyRegistry())
	res, err := flow.Inspect(t.Context())
	if err != nil {
		t.Fatalf("Inspect() error = %v", err)
	}
	if res.Verdict != VerdictReady {
		t.Fatalf("verdict = %v, want ready; issues: %+v", res.Verdict, res.Issues())
	}
}

func TestInspectNormalMachineIsBlocked(t *testing.T) {
	reg := fake.NewRegistry().
		SetString(platform.HKLM, `SOFTWARE\Microsoft\Windows\CurrentVersion\Setup\State`, "StateName", "IMAGE_STATE_COMPLETE")
	flow, _, _, _ := newTestFlowWithRegistry(t, reg)
	res, err := flow.Inspect(t.Context())
	if err != nil {
		t.Fatalf("Inspect() error = %v", err)
	}
	if res.Verdict != VerdictBlocked {
		t.Fatalf("verdict = %v, want blocked for a deployed installation", res.Verdict)
	}
}

func TestInspectAccountFailureBlocks(t *testing.T) {
	flow, runner, _, _ := newTestFlowWithRegistry(t, auditReadyRegistry())
	runner.SetResult("powershell.exe", platform.CommandResult{ExitCode: 1})
	res, err := flow.Inspect(t.Context())
	if err != nil {
		t.Fatalf("Inspect() error = %v", err)
	}
	if res.Verdict != VerdictBlocked {
		t.Fatalf("verdict = %v, want blocked when the account list is unavailable", res.Verdict)
	}
}

func TestInspectDetectsExtraAccounts(t *testing.T) {
	flow, runner, _, _ := newTestFlowWithRegistry(t, auditReadyRegistry())
	runner.SetResult("powershell.exe", platform.CommandResult{
		Stdout: `{"accounts":[{"Name":"customer","SID":"S-1-5-21-1-2-3-1001","Disabled":false},{"Name":"Administrator","SID":"S-1-5-21-1-2-3-500","Disabled":false}]}`,
	})
	res, err := flow.Inspect(t.Context())
	if err != nil {
		t.Fatalf("Inspect() error = %v", err)
	}
	if res.Verdict != VerdictAttention {
		t.Fatalf("verdict = %v, want attention with one extra account", res.Verdict)
	}
	c := checkByID(res.Checks, "accounts")
	if c == nil || !contains(c.Detail, "customer") {
		t.Errorf("accounts check should name the extra account, got %+v", c)
	}
}

func TestInspectReadsPendingReboot(t *testing.T) {
	reg := auditReadyRegistry().AddKey(platform.HKLM,
		`SOFTWARE\Microsoft\Windows\CurrentVersion\Component Based Servicing\RebootPending`)
	flow, _, _, _ := newTestFlowWithRegistry(t, reg)
	res, err := flow.Inspect(t.Context())
	if err != nil {
		t.Fatalf("Inspect() error = %v", err)
	}
	if res.Verdict != VerdictBlocked {
		t.Fatalf("verdict = %v, want blocked with CBS reboot pending", res.Verdict)
	}
}

func TestInspectDetectsRunningSysprep(t *testing.T) {
	flow, _, sp, _ := newTestFlowWithRegistry(t, auditReadyRegistry())
	flow.Procs = &fake.ProcessLister{Names: []string{"sysprep.exe", "winprep.exe"}}
	res, err := flow.Inspect(t.Context())
	if err != nil {
		t.Fatalf("Inspect() error = %v", err)
	}
	if res.Verdict != VerdictBlocked {
		t.Fatalf("verdict = %v, want blocked while sysprep is running", res.Verdict)
	}
	if sp.ressealed {
		t.Error("inspect must never invoke sysprep")
	}
}

func TestResealSuccessAndFailures(t *testing.T) {
	flow, _, sp, _ := newTestFlowWithRegistry(t, auditReadyRegistry())

	// Success: exit code 0 means initiated, and the wording must not claim
	// final verification.
	sp.result = platform.CommandResult{ExitCode: 0}
	res, err := flow.Reseal(t.Context())
	if err != nil || res.Outcome != ResealInitiated {
		t.Fatalf("Reseal() = %+v, %v; want initiated", res, err)
	}
	if !contains(res.Detail, "first-time setup") {
		t.Error("success detail should mention first-time setup")
	}

	// Sysprep error: exit code non-zero.
	sp.result = platform.CommandResult{ExitCode: 31}
	res, _ = flow.Reseal(t.Context())
	if res.Outcome != ResealSysprepError || res.ExitCode != 31 {
		t.Fatalf("Reseal() = %+v; want sysprep error with code 31", res)
	}

	// Launch failure.
	sp.runErr = errors.New("access is denied")
	res, _ = flow.Reseal(t.Context())
	if res.Outcome != ResealLaunchFailed {
		t.Fatalf("Reseal() = %+v; want launch failed", res)
	}

	// Missing sysprep.
	sp.pathErr = errors.New("not found")
	res, _ = flow.Reseal(t.Context())
	if res.Outcome != ResealLaunchFailed {
		t.Fatalf("Reseal() = %+v; want launch failed when sysprep is missing", res)
	}
}

func TestResealNeverInvokedDuringDryRun(t *testing.T) {
	flow, runner, sp, store := newTestFlowWithRegistry(t, auditReadyRegistry())
	// A dry run is exactly Inspect + reporting: no cleanup, no sysprep, and
	// nothing written.
	before, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := flow.Inspect(t.Context()); err != nil {
		t.Fatal(err)
	}
	if sp.ressealed {
		t.Error("dry-run inspection invoked sysprep")
	}
	if runner.Calls() > 1 {
		t.Errorf("inspection ran %d commands; expected only the account query", runner.Calls())
	}
	after, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(after.Entries) != len(before.Entries) {
		t.Error("dry-run modified the artifact manifest")
	}
	if _, err := os.Stat(store.Path()); !os.IsNotExist(err) {
		t.Error("dry-run created a manifest file")
	}
}

func TestInspectUsesFreshStateEveryTime(t *testing.T) {
	flow, _, _, _ := newTestFlowWithRegistry(t, auditReadyRegistry())
	res, _ := flow.Inspect(t.Context())
	if res.Verdict != VerdictReady {
		t.Fatalf("first inspection = %v, want ready", res.Verdict)
	}
	// The machine "leaves" audit mode between calls; a second inspection must
	// notice rather than reuse stale state.
	flow.Reg.(*fake.Registry).SetString(platform.HKLM,
		`SOFTWARE\Microsoft\Windows\CurrentVersion\Setup\State`, "StateName", "IMAGE_STATE_COMPLETE")
	res, err := flow.Inspect(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if res.Verdict != VerdictBlocked {
		t.Fatalf("second inspection = %v, want blocked after state changed", res.Verdict)
	}
}
