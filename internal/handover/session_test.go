package handover

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/developerasaad/winprep/internal/artifacts"
	"github.com/developerasaad/winprep/internal/audit"
	"github.com/developerasaad/winprep/internal/platform"
	"github.com/developerasaad/winprep/internal/platform/fake"
)

// fakeSysprep records whether the reseal was attempted.
type fakeSysprep struct {
	path      string
	pathErr   error
	exitCode  int
	runErr    error
	ressealed bool
}

func (s *fakeSysprep) SysprepPath() (string, error) { return s.path, s.pathErr }

func (s *fakeSysprep) RunReseal(context.Context) (int, error) {
	s.ressealed = true
	return s.exitCode, s.runErr
}

// auditReadyRegistry returns a registry that reports a consistent Audit Mode.
func auditReadyRegistry() *fake.Registry {
	return fake.NewRegistry().
		SetString(platform.HKLM, `SOFTWARE\Microsoft\Windows\CurrentVersion\Setup\State`, "StateName", "IMAGE_STATE_UNDEPLOYABLE").
		SetString(platform.HKLM, `SYSTEM\Setup`, "CmdLine", "audit.exe")
}

// newTestController builds a controller whose machine is a plausible Audit
// Mode laptop with a registered demo profile. The returned registry handle
// lets tests mutate the machine between phases.
func newTestController(t *testing.T, reg *fake.Registry) (*Controller, *fake.Registry, *fake.System, *fakeSysprep, *artifacts.Store) {
	t.Helper()
	c, sys, sp, store, demoRoot := buildControllerParts(t, reg)
	reg.SetString(platform.HKLM, profileListKey+`\`+testSID, "ProfileImagePath", demoRoot)
	reg.SetString(platform.HKU, testSID+`\`+userShellFolders, downloadsValue, filepath.Join(demoRoot, "Downloads"))
	return c, reg, sys, sp, store
}

// buildControllerParts assembles the controller pieces without registering a
// demo profile, for tests that need an empty registry.
func buildControllerParts(t *testing.T, reg *fake.Registry) (c *Controller, sys *fake.System, sp *fakeSysprep, store *artifacts.Store, demoRoot string) {
	t.Helper()
	store = artifacts.NewStore(t.TempDir())
	sys = &fake.System{
		Sys: platform.SystemInfo{
			OSName: "Windows 11 Pro", Build: "10.0.22631.4317",
			InstallationType: "Client", Architecture: "x64", IsWindows: true,
		},
		Usr: platform.UserInfo{IsElevated: true, SID: testSID, IsBuiltinAdmin: true},
		Accounts: []platform.LocalAccount{
			{Name: "Administrator", SID: testSID, RID: 500},
		},
	}
	sp = &fakeSysprep{path: `C:\Windows\System32\Sysprep\Sysprep.exe`}
	logs := &fake.SysprepMonitor{}
	procs := &fake.ProcessLister{Names: []string{"winprep.exe"}}

	// The demo profile and its standard folders live under the profiles root.
	usersDir := filepath.Join(t.TempDir(), "Users")
	demoRoot = filepath.Join(usersDir, "Administrator")
	for _, d := range []string{
		filepath.Join(demoRoot, "Downloads"),
		filepath.Join(demoRoot, "AppData", "Local", "Temp"),
	} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	c = &Controller{
		Sys:            sys,
		Reg:            reg,
		Procs:          procs,
		Audit:          audit.NewService(reg, sys),
		Store:          store,
		Sysprep:        sp,
		SysprepReports: logs,
		UsersDir:       usersDir,
	}
	return c, sys, sp, store, demoRoot
}

func TestBeginPlansTheCleanupForAResultMachine(t *testing.T) {
	c, _, _, _, store := newTestController(t, auditReadyRegistry())
	session, err := c.Begin(context.Background())
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	if session.Preflight.Verdict != VerdictReady {
		t.Fatalf("verdict = %v, want Ready; issues: %+v", session.Preflight.Verdict, session.Preflight.Issues())
	}
	if session.Plan == nil {
		t.Fatal("a ready machine must come with a cleanup plan")
	}
	if !session.Plan.HasActions() {
		t.Error("the data directory target should always be present while the store dir exists")
	}
	if _, err := os.Stat(store.Dir); err != nil {
		t.Errorf("Begin must not modify anything, store dir error: %v", err)
	}
}

func TestBeginIsIdempotent(t *testing.T) {
	c, _, _, _, _ := newTestController(t, auditReadyRegistry())
	s1, err := c.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	s2, err := c.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !sameScope(*s1.Plan, *s2.Plan) {
		t.Error("repeated Begin must produce the same plan scope")
	}
	if s1.Preflight.Verdict != s2.Preflight.Verdict {
		t.Error("repeated Begin must produce the same verdict")
	}
}

// Test A (mandatory regression): cancelling must not modify anything.
func TestCancellationModifiesNothing(t *testing.T) {
	c, _, _, sp, _ := newTestController(t, auditReadyRegistry())
	demoFile := filepath.Join(c.UsersDir, "Administrator", "Downloads", "customer-notes.txt")
	if err := os.WriteFile(demoFile, []byte("demo"), 0o644); err != nil {
		t.Fatal(err)
	}

	session, err := c.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	// The operator declines: Approve is never called. A presentation bug that
	// calls Execute anyway must still be refused.
	res, err := session.Execute(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.Phase != PhaseCancelled {
		t.Fatalf("phase = %v, want PhaseCancelled", res.Phase)
	}
	if data, err := os.ReadFile(demoFile); err != nil || string(data) != "demo" {
		t.Errorf("the demo file must be untouched, got %q, %v", data, err)
	}
	if _, err := os.Stat(c.Store.Dir); err != nil {
		t.Error("the WinPrep data directory must still exist")
	}
	if sp.ressealed {
		t.Error("an unapproved session must never invoke Sysprep")
	}
}

// Test E (mandatory regression): a failed cleanup must block the reseal.
func TestCleanupFailureBlocksResealing(t *testing.T) {
	c, _, _, sp, _ := newTestController(t, auditReadyRegistry())
	downloads := filepath.Join(c.UsersDir, "Administrator", "Downloads")
	if err := os.WriteFile(filepath.Join(downloads, "locked.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	session, err := c.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	session.Approve()

	// The folder becomes read-only after planning, so removals fail during
	// execution - simulating a program holding the files open.
	if err := os.Chmod(downloads, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(downloads, 0o700) })

	res, err := session.Execute(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.Phase != PhaseCleanupFailed {
		t.Fatalf("phase = %v, want PhaseCleanupFailed", res.Phase)
	}
	if sp.ressealed {
		t.Error("an incomplete cleanup must never start Sysprep")
	}
	if !strings.Contains(strings.Join(res.SummaryLines(), "\n"), "reseal was NOT started") {
		t.Error("the report must say the reseal was not started")
	}

	// After the condition clears, a fresh session completes the job.
	if err := os.Chmod(downloads, 0o700); err != nil {
		t.Fatal(err)
	}
	session2, err := c.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	session2.Approve()
	res2, err := session2.Execute(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res2.Phase != PhaseResealInitiated {
		t.Fatalf("retry phase = %v, want PhaseResealInitiated: %+v", res2.Phase, res2)
	}
}

// Test G (mandatory regression): an active Sysprep operation is protected.
func TestActiveSysprepIsNeverOverlapped(t *testing.T) {
	c, _, _, sp, _ := newTestController(t, auditReadyRegistry())

	// A sysprep.exe process with freshly written logs is an active operation:
	// the inspection must block.
	c.Procs = &fake.ProcessLister{Names: []string{"sysprep.exe"}}
	c.SysprepReports.(*fake.SysprepMonitor).LogTimes = []time.Time{time.Now()}
	res, err := c.Inspect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.Verdict != VerdictBlocked {
		t.Fatalf("an active Sysprep operation must block, verdict = %v", res.Verdict)
	}
	session, err := c.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	session.Approve()
	xres, err := session.Execute(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if xres.Phase == PhaseResealInitiated {
		t.Fatal("a blocked session must never reseal")
	}
	if sp.ressealed {
		t.Error("a blocked session must never invoke Sysprep")
	}
}

// Test G continued: a Sysprep process that appears after planning stops the
// execution gate before anything is modified.
func TestSysprepAppearingAfterPlanningStopsExecution(t *testing.T) {
	c, _, _, sp, _ := newTestController(t, auditReadyRegistry())
	session, err := c.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	session.Approve()

	// The idle System Preparation Tool window opens between confirmation and
	// execution.
	c.Procs = &fake.ProcessLister{Names: []string{"sysprep.exe"}}

	res, err := session.Execute(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.Phase != PhaseConditionsChanged {
		t.Fatalf("phase = %v, want PhaseConditionsChanged", res.Phase)
	}
	if !strings.Contains(res.Detail, "System Preparation Tool window is still open") {
		t.Errorf("the refusal should explain the window, detail = %q", res.Detail)
	}
	if sp.ressealed {
		t.Error("the gate must never launch Sysprep alongside another instance")
	}
	if entries, _ := os.ReadDir(filepath.Join(c.UsersDir, "Administrator", "Downloads")); true {
		_ = entries // nothing to assert beyond no-reseal; file checks live below
	}
}

// Test H (mandatory regression): a normally deployed Windows installation is
// rejected instead of being reset through Sysprep.
func TestDeployedWindowsIsRejected(t *testing.T) {
	reg := fake.NewRegistry().
		SetString(platform.HKLM, `SOFTWARE\Microsoft\Windows\CurrentVersion\Setup\State`, "StateName", "IMAGE_STATE_COMPLETE")
	c, _, _, sp, _ := newTestController(t, reg)

	session, err := c.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if session.Preflight.Verdict != VerdictBlocked {
		t.Fatalf("a deployed installation must be blocked, verdict = %v", session.Preflight.Verdict)
	}
	if session.Plan != nil {
		t.Error("a blocked machine must not produce a cleanup plan")
	}
	session.Approve()
	res, err := session.Execute(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.Phase != PhaseCancelled {
		t.Fatalf("phase = %v, want PhaseCancelled for a blocked session", res.Phase)
	}
	if sp.ressealed {
		t.Error("WinPrep must never misuse Sysprep as a general reset")
	}
}

func TestExecuteHappyPathRunsTheApprovedSequence(t *testing.T) {
	c, _, _, sp, _ := newTestController(t, auditReadyRegistry())
	downloads := filepath.Join(c.UsersDir, "Administrator", "Downloads")
	if err := os.WriteFile(filepath.Join(downloads, "demo.pdf"), []byte("demo"), 0o644); err != nil {
		t.Fatal(err)
	}

	session, err := c.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	session.Approve()
	res, err := session.Execute(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.Phase != PhaseResealInitiated {
		t.Fatalf("phase = %v, want PhaseResealInitiated: %s", res.Phase, strings.Join(res.SummaryLines(), "\n"))
	}
	if !sp.ressealed {
		t.Error("an approved session must invoke Sysprep")
	}
	if entries, _ := os.ReadDir(downloads); len(entries) != 0 {
		t.Errorf("the approved demo data must be removed, %d entries remain", len(entries))
	}
	if _, err := os.Stat(c.Store.Dir); !os.IsNotExist(err) {
		t.Error("the WinPrep data directory must be removed")
	}
}

func TestExecuteRefusesWhenTheScopeChanged(t *testing.T) {
	c, _, _, sp, _ := newTestController(t, auditReadyRegistry())
	downloads := filepath.Join(c.UsersDir, "Administrator", "Downloads")
	if err := os.WriteFile(filepath.Join(downloads, "demo.pdf"), []byte("demo"), 0o644); err != nil {
		t.Fatal(err)
	}

	session, err := c.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	session.Approve()

	// Edge gets used between the confirmation and the execution: the approved
	// plan did not include its browsing data, so the scope grew.
	edgeCache := filepath.Join(c.UsersDir, "Administrator", "AppData", "Local", "Microsoft", "Edge", "User Data", "Default", "Cache")
	if err := os.MkdirAll(edgeCache, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(edgeCache, "f"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	res, err := session.Execute(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.Phase != PhaseConditionsChanged {
		t.Fatalf("phase = %v, want PhaseConditionsChanged", res.Phase)
	}
	if sp.ressealed {
		t.Error("a changed scope must stop the handover")
	}
	// Nothing was modified: the original demo file must still exist.
	if _, err := os.Stat(filepath.Join(downloads, "demo.pdf")); err != nil {
		t.Errorf("nothing should have been deleted, demo file error: %v", err)
	}
}

func TestExecuteStopsWhenAuditModeIsLost(t *testing.T) {
	c, reg, _, sp, _ := newTestController(t, auditReadyRegistry())
	session, err := c.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	session.Approve()

	// The machine leaves Audit Mode between confirmation and execution.
	reg.SetString(platform.HKLM,
		`SOFTWARE\Microsoft\Windows\CurrentVersion\Setup\State`, "StateName", "IMAGE_STATE_COMPLETE")

	res, err := session.Execute(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.Phase != PhaseConditionsChanged {
		t.Fatalf("phase = %v, want PhaseConditionsChanged", res.Phase)
	}
	if sp.ressealed {
		t.Error("a machine that left Audit Mode must not be resealed with the old approval")
	}
}

func TestExecuteReportsBusyWhenAnotherInstanceRuns(t *testing.T) {
	c, _, _, sp, _ := newTestController(t, auditReadyRegistry())
	c.Lock = &fake.InstanceLock{Held: true}

	session, err := c.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	session.Approve()
	res, err := session.Execute(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.Phase != PhaseBusy {
		t.Fatalf("phase = %v, want PhaseBusy", res.Phase)
	}
	if sp.ressealed {
		t.Error("a busy machine must not be resealed twice")
	}
}

func TestResealSuccessAndFailures(t *testing.T) {
	c, _, _, sp, _ := newTestController(t, auditReadyRegistry())

	// Success: exit code 0 means initiated, and the wording must not claim
	// final verification.
	sp.exitCode = 0
	res, err := c.Reseal(context.Background())
	if err != nil || res.Outcome != ResealInitiated {
		t.Fatalf("Reseal() = %+v, %v; want initiated", res, err)
	}
	if !contains(res.Detail, "first-time setup") {
		t.Error("success detail should mention first-time setup")
	}

	// Sysprep error: exit code non-zero.
	sp.exitCode = 31
	res, _ = c.Reseal(context.Background())
	if res.Outcome != ResealSysprepError || res.ExitCode != 31 {
		t.Fatalf("Reseal() = %+v; want sysprep error with code 31", res)
	}

	// Launch failure.
	sp.runErr = errors.New("access is denied")
	res, _ = c.Reseal(context.Background())
	if res.Outcome != ResealLaunchFailed {
		t.Fatalf("Reseal() = %+v; want launch failed", res)
	}

	// Missing sysprep.
	sp.pathErr = errors.New("not found")
	res, _ = c.Reseal(context.Background())
	if res.Outcome != ResealLaunchFailed {
		t.Fatalf("Reseal() = %+v; want launch failed when sysprep is missing", res)
	}
}

func TestInspectNormalMachineIsBlocked(t *testing.T) {
	reg := fake.NewRegistry().
		SetString(platform.HKLM, `SOFTWARE\Microsoft\Windows\CurrentVersion\Setup\State`, "StateName", "IMAGE_STATE_COMPLETE")
	c, _, _, _, _ := newTestController(t, reg)
	res, err := c.Inspect(context.Background())
	if err != nil {
		t.Fatalf("Inspect() error = %v", err)
	}
	if res.Verdict != VerdictBlocked {
		t.Fatalf("verdict = %v, want blocked for a deployed installation", res.Verdict)
	}
}

func TestInspectAccountFailureBlocks(t *testing.T) {
	c, _, sys, _, _ := newTestController(t, auditReadyRegistry())
	sys.AccountsErr = errors.New("access denied")
	res, err := c.Inspect(context.Background())
	if err != nil {
		t.Fatalf("Inspect() error = %v", err)
	}
	if res.Verdict != VerdictBlocked {
		t.Fatalf("verdict = %v, want blocked when the account list is unavailable", res.Verdict)
	}
}

func TestInspectDetectsExtraAccountsAsBlocker(t *testing.T) {
	c, _, sys, _, _ := newTestController(t, auditReadyRegistry())
	sys.Accounts = append(sys.Accounts,
		platform.LocalAccount{Name: "customer", SID: "S-1-5-21-1-2-3-1001", RID: 1001})
	res, err := c.Inspect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.Verdict != VerdictBlocked {
		t.Fatalf("verdict = %v, want blocked with one extra account", res.Verdict)
	}
	c2 := checkByID(res.Checks, "accounts")
	if c2 == nil || !contains(c2.Detail, "customer") {
		t.Errorf("accounts check should name the extra account, got %+v", c2)
	}
}

func TestInspectReadsPendingReboot(t *testing.T) {
	reg := auditReadyRegistry().AddKey(platform.HKLM,
		`SOFTWARE\Microsoft\Windows\CurrentVersion\Component Based Servicing\RebootPending`)
	c, _, _, _, _ := newTestController(t, reg)
	res, err := c.Inspect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.Verdict != VerdictBlocked {
		t.Fatalf("verdict = %v, want blocked with CBS reboot pending", res.Verdict)
	}
}

func TestInspectUsesFreshStateEveryTime(t *testing.T) {
	c, reg, _, _, _ := newTestController(t, auditReadyRegistry())
	res, _ := c.Inspect(context.Background())
	if res.Verdict != VerdictReady {
		t.Fatalf("first inspection = %v, want ready", res.Verdict)
	}
	// The machine "leaves" audit mode between calls; a second inspection must
	// notice rather than reuse stale state.
	reg.SetString(platform.HKLM,
		`SOFTWARE\Microsoft\Windows\CurrentVersion\Setup\State`, "StateName", "IMAGE_STATE_COMPLETE")
	res, err := c.Inspect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.Verdict != VerdictBlocked {
		t.Fatalf("second inspection = %v, want blocked after state changed", res.Verdict)
	}
}

func TestIdleSysprepWindowIsAttentionNotBlocker(t *testing.T) {
	c, _, _, _, _ := newTestController(t, auditReadyRegistry())
	c.Procs = &fake.ProcessLister{Names: []string{"sysprep.exe", "winprep.exe"}}
	// Stale logs: the window has been sitting idle since the machine was
	// manufactured.
	c.SysprepReports.(*fake.SysprepMonitor).LogTimes = []time.Time{time.Now().Add(-72 * time.Hour)}

	res, err := c.Inspect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.Verdict != VerdictAttention {
		t.Fatalf("an idle Sysprep window should be attention, verdict = %v", res.Verdict)
	}
}
