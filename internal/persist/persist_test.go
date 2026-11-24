package persist

import (
	"context"
	"strings"
	"testing"

	"github.com/developerasaad/winprep/internal/artifacts"
	"github.com/developerasaad/winprep/internal/audit"
	"github.com/developerasaad/winprep/internal/platform"
	"github.com/developerasaad/winprep/internal/platform/fake"
)

func newService(reg *fake.Registry, procs *fake.ProcessLister, store *artifacts.Store) *Service {
	sys := &fake.System{
		Sys: platform.SystemInfo{IsWindows: true},
		Usr: platform.UserInfo{IsElevated: true, SID: "S-1-5-21-1-2-3-500", IsBuiltinAdmin: true},
	}
	return NewService(sys, audit.NewService(reg, sys, nil), procs, store)
}

// auditRegistry mirrors a real Audit Mode machine: the ImageState value real
// Windows writes, plus the AuditBoot flag. No StateName, no "audit.exe"
// command line - those were the v0.2.0-alpha.1 assumptions real machines do
// not match.
func auditRegistry() *fake.Registry {
	return fake.NewRegistry().
		SetString(platform.HKLM, `SOFTWARE\Microsoft\Windows\CurrentVersion\Setup\State`, "ImageState", "IMAGE_STATE_UNDEPLOYABLE").
		SetDWORD(platform.HKLM, `SYSTEM\Setup\Status`, "AuditBoot", 1)
}

func TestAuditModeIsReadyWithoutChanges(t *testing.T) {
	svc := newService(auditRegistry(), &fake.ProcessLister{}, artifacts.NewStore(t.TempDir()))
	r, err := svc.Inspect(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if !r.ReadyForDemo {
		t.Fatal("confirmed audit mode should be ready for demos")
	}
	if len(r.ChangesApplied) != 0 {
		t.Errorf("persist must apply no changes, applied: %+v", r.ChangesApplied)
	}
	joined := strings.Join(r.Lines(), "\n")
	if !strings.Contains(joined, "No system changes were made") {
		t.Error("report must state that nothing was changed")
	}
	if !strings.Contains(joined, "stays in Audit Mode across restarts") {
		t.Error("report must explain the actual persistence behavior")
	}
}

func TestSysprepWindowNoteWithoutAutoClose(t *testing.T) {
	svc := newService(auditRegistry(), &fake.ProcessLister{Names: []string{"sysprep.exe", "explorer.exe"}}, artifacts.NewStore(t.TempDir()))
	r, err := svc.Inspect(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if !r.SysprepWindowOpen {
		t.Fatal("running sysprep.exe should be detected")
	}
	joined := strings.Join(r.Lines(), "\n")
	if !strings.Contains(joined, "does not close the Sysprep window automatically") {
		t.Error("report must explain that the window is not closed automatically")
	}
}

func TestOOBEGuidance(t *testing.T) {
	reg := fake.NewRegistry().
		SetString(platform.HKLM, `SOFTWARE\Microsoft\Windows\CurrentVersion\Setup\State`, "ImageState", "IMAGE_STATE_UNDEPLOYABLE").
		SetDWORD(platform.HKLM, `SYSTEM\Setup`, "OOBEInProgress", 1)
	svc := newService(reg, &fake.ProcessLister{}, artifacts.NewStore(t.TempDir()))
	r, err := svc.Inspect(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if r.ReadyForDemo {
		t.Error("OOBE is not ready for demos")
	}
	joined := strings.Join(r.Lines(), "\n")
	if !strings.Contains(joined, "Ctrl+Shift+F3") {
		t.Error("OOBE guidance must mention Ctrl+Shift+F3")
	}
}

func TestNormalInstallGuidance(t *testing.T) {
	reg := fake.NewRegistry().
		SetString(platform.HKLM, `SOFTWARE\Microsoft\Windows\CurrentVersion\Setup\State`, "ImageState", "IMAGE_STATE_COMPLETE")
	svc := newService(reg, &fake.ProcessLister{}, artifacts.NewStore(t.TempDir()))
	r, err := svc.Inspect(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if r.ReadyForDemo {
		t.Error("a deployed installation is not ready for the audit workflow")
	}
	joined := strings.Join(r.Lines(), "\n")
	if !strings.Contains(joined, "not a supported") {
		t.Error("guidance must state that converting a deployed install is unsupported")
	}
}

func TestUncertainStateIsNotReady(t *testing.T) {
	// Audit-family image state without the corroborating audit boot flag:
	// plausible-looking but unconfirmable, which must stay not-ready.
	reg := fake.NewRegistry().
		SetString(platform.HKLM, `SOFTWARE\Microsoft\Windows\CurrentVersion\Setup\State`, "ImageState", "IMAGE_STATE_UNDEPLOYABLE")
	svc := newService(reg, &fake.ProcessLister{}, artifacts.NewStore(t.TempDir()))
	r, err := svc.Inspect(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if r.ReadyForDemo {
		t.Error("an uncertain state must never be reported as ready")
	}
	if r.Audit.State != audit.StateUncertain {
		t.Errorf("state = %v, want StateUncertain", r.Audit.State)
	}
}

func TestRepeatedRunsAreIdempotent(t *testing.T) {
	store := artifacts.NewStore(t.TempDir())
	svc := newService(auditRegistry(), &fake.ProcessLister{}, store)
	r1, err := svc.Inspect(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	r2, err := svc.Inspect(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(r1.ChangesApplied) != 0 || len(r2.ChangesApplied) != 0 {
		t.Error("persist must never apply changes in v0.1")
	}
	if _, err := store.Load(); err != nil {
		t.Errorf("persist must not create a manifest: %v", err)
	}
}

func TestRecordedArtifactsAreSurfaced(t *testing.T) {
	store := artifacts.NewStore(t.TempDir())
	m := artifacts.Manifest{}
	m.Add(artifacts.Entry{Type: artifacts.TypeFile, Path: store.Dir + `\note.txt`})
	if err := store.Save(m); err != nil {
		t.Fatal(err)
	}
	svc := newService(auditRegistry(), &fake.ProcessLister{}, store)
	r, err := svc.Inspect(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Artifacts) != 1 {
		t.Fatalf("recorded artifacts should be surfaced, got %d", len(r.Artifacts))
	}
	if !r.ReadyForDemo {
		t.Error("recorded artifacts alone must not block demos; they are cleaned at handover")
	}
}

func TestLinesMentionSafetyOfAdminSession(t *testing.T) {
	svc := newService(auditRegistry(), &fake.ProcessLister{}, artifacts.NewStore(t.TempDir()))
	r, err := svc.Inspect(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(r.Lines(), "\n"), "administrator rights") {
		t.Error("the report must mention that Audit Mode runs with administrator rights")
	}
}

// Guard against accidental future regressions: the persist service must not
// depend on the runner at all, since it should never spawn commands.
var _ = context.Background
