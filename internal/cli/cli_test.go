package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/developerasaad/winprep/internal/apptest"
)

// newIO returns a CLI IO pair with captured output.
func newIO(in string, interactive bool) (IO, *bytes.Buffer, *bytes.Buffer) {
	var out, errOut bytes.Buffer
	return IO{
		Out:         &out,
		Err:         &errOut,
		In:          strings.NewReader(in),
		Interactive: interactive,
	}, &out, &errOut
}

func TestNoArgumentsShowsHelp(t *testing.T) {
	f := apptest.New(t.TempDir())
	io, out, _ := newIO("", false)
	code := Run(t.Context(), f.App, io, nil)
	if code != ExitOK {
		t.Errorf("exit = %d, want %d", code, ExitOK)
	}
	if !strings.Contains(out.String(), "Usage:") {
		t.Error("no-args run should print help")
	}
}

func TestHelpVariants(t *testing.T) {
	f := apptest.New(t.TempDir())
	for _, cmd := range []string{"help", "--help", "-h", "/?"} {
		io, out, _ := newIO("", false)
		if code := Run(t.Context(), f.App, io, []string{cmd}); code != ExitOK {
			t.Errorf("%s: exit = %d, want %d", cmd, code, ExitOK)
		}
		if !strings.Contains(out.String(), "winprep clean --dry-run") {
			t.Errorf("%s: help should document the dry-run option", cmd)
		}
	}
}

func TestVersion(t *testing.T) {
	f := apptest.New(t.TempDir())
	for _, cmd := range []string{"version", "--version", "-v"} {
		io, out, _ := newIO("", false)
		if code := Run(t.Context(), f.App, io, []string{cmd}); code != ExitOK {
			t.Errorf("%s: exit = %d", cmd, code)
		}
		s := out.String()
		if !strings.Contains(s, "winprep 0.1.0") {
			t.Errorf("%s: version output = %q", cmd, s)
		}
		if !strings.Contains(s, "linux/amd64") && !strings.Contains(s, "windows/amd64") {
			t.Errorf("%s: version output should name the build target: %q", cmd, s)
		}
	}
}

func TestUnknownCommandIsUsageError(t *testing.T) {
	f := apptest.New(t.TempDir())
	io, _, errOut := newIO("", false)
	code := Run(t.Context(), f.App, io, []string{"explode"})
	if code != ExitUsage {
		t.Errorf("exit = %d, want %d", code, ExitUsage)
	}
	if !strings.Contains(errOut.String(), "Unknown command: explode") {
		t.Errorf("stderr = %q", errOut.String())
	}
}

func TestPersistExplainsAuditMode(t *testing.T) {
	f := apptest.New(t.TempDir())
	io, out, _ := newIO("", false)
	if code := Run(t.Context(), f.App, io, []string{"persist"}); code != ExitOK {
		t.Fatalf("exit = %d", code)
	}
	if !strings.Contains(out.String(), "ready to be demonstrated") {
		t.Error("persist should report the laptop as demonstrable")
	}
}

func TestCleanDryRunChangesNothing(t *testing.T) {
	f := apptest.New(t.TempDir())
	io, out, _ := newIO("", false)
	code := Run(t.Context(), f.App, io, []string{"clean", "--dry-run"})
	if code != ExitOK {
		t.Fatalf("exit = %d", code)
	}
	s := out.String()
	if !strings.Contains(s, "Dry run: no changes were made") {
		t.Error("dry run must state that nothing was changed")
	}
	if !strings.Contains(s, "A handover appears possible") {
		t.Error("dry run should explain whether a handover is possible")
	}
	if f.Sysprep.Called {
		t.Error("dry run must never invoke Sysprep")
	}
	if f.Run.Calls() != 0 {
		t.Error("the inspection must not spawn any processes")
	}
}

func TestCleanBlockedStopsBeforeConfirmation(t *testing.T) {
	f := apptest.New(t.TempDir())
	f.Sys.Usr.IsElevated = false // block: no admin privileges
	io, out, _ := newIO("y\n", true)
	code := Run(t.Context(), f.App, io, []string{"clean", "--yes"})
	if code != ExitFail {
		t.Errorf("exit = %d, want %d", code, ExitFail)
	}
	if !strings.Contains(out.String(), "cannot proceed") {
		t.Error("blocked handover must be explained")
	}
	if f.Sysprep.Called {
		t.Error("a blocked handover must never start Sysprep, even with --yes")
	}
}

func TestCleanNoninteractiveRequiresYes(t *testing.T) {
	f := apptest.New(t.TempDir())
	io, out, _ := newIO("", false)
	code := Run(t.Context(), f.App, io, []string{"clean"})
	if code != ExitFail {
		t.Errorf("exit = %d, want %d", code, ExitFail)
	}
	s := out.String()
	if !strings.Contains(s, "winprep clean --yes") {
		t.Error("noninteractive clean should point at the explicit confirmation")
	}
	if !strings.Contains(s, "--yes does not bypass them") {
		t.Error("the hint must say that safety checks still apply")
	}
	if f.Sysprep.Called {
		t.Error("no confirmation means no Sysprep")
	}
}

func TestCleanInteractiveDecline(t *testing.T) {
	f := apptest.New(t.TempDir())
	io, out, _ := newIO("n\n", true)
	code := Run(t.Context(), f.App, io, []string{"clean"})
	if code != ExitFail {
		t.Errorf("exit = %d, want %d", code, ExitFail)
	}
	if !strings.Contains(out.String(), "Cancelled") {
		t.Error("declined clean should say it was cancelled")
	}
	if f.Sysprep.Called {
		t.Error("a declined handover must not start Sysprep")
	}
}

func TestCleanConfirmedRunsSysprep(t *testing.T) {
	f := apptest.New(t.TempDir())
	io, out, _ := newIO("", false)
	code := Run(t.Context(), f.App, io, []string{"clean", "--yes"})
	if code != ExitOK {
		t.Fatalf("exit = %d, output:\n%s", code, out.String())
	}
	s := out.String()
	if !strings.Contains(s, "Reseal initiated") {
		t.Error("a successful reseal should be reported as initiated")
	}
	if !strings.Contains(s, "Verify this before selling") {
		t.Error("the result must tell the operator to verify the outcome")
	}
	if !f.Sysprep.Called {
		t.Error("confirmed handover should invoke Sysprep")
	}
}

func TestCleanSysprepErrorShowsLogs(t *testing.T) {
	f := apptest.New(t.TempDir())
	f.Sysprep.ExitCode = 1
	io, out, _ := newIO("", false)
	code := Run(t.Context(), f.App, io, []string{"clean", "--yes"})
	if code != ExitFail {
		t.Errorf("exit = %d, want %d", code, ExitFail)
	}
	s := out.String()
	if !strings.Contains(s, "Sysprep reported an error") {
		t.Error("a failing Sysprep must be reported as an error")
	}
	if !strings.Contains(s, "Sysprep\\Panther\\setupact.log") {
		t.Error("the log locations must be shown")
	}
}

func TestCleanUnknownOptionIsUsageError(t *testing.T) {
	f := apptest.New(t.TempDir())
	io, _, errOut := newIO("", false)
	code := Run(t.Context(), f.App, io, []string{"clean", "--force"})
	if code != ExitUsage {
		t.Errorf("exit = %d, want %d", code, ExitUsage)
	}
	if !strings.Contains(errOut.String(), "unknown option for clean: --force") {
		t.Errorf("stderr = %q", errOut.String())
	}
}

// Test B (mandatory regression): a CLI-approved Clean & Finish must drive the
// shared controller to the same end state as the TUI flow, which the
// model tests drive through the very same session. File-level effects are
// asserted here so "same workflow" means the same deletions, not just the
// same exit path.
func TestCleanConfirmedRemovesApprovedData(t *testing.T) {
	f := apptest.New(t.TempDir())
	downloads := f.DemoProfile + string(os.PathSeparator) + "Downloads"
	if err := os.WriteFile(filepath.Join(downloads, "customer-file.txt"), []byte("demo data"), 0o644); err != nil {
		t.Fatal(err)
	}
	// A file on the Desktop is outside the approved scope and must survive.
	desktop := f.DemoProfile + string(os.PathSeparator) + "Desktop"
	if err := os.MkdirAll(desktop, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(desktop, "keep-me.txt"), []byte("operator file"), 0o644); err != nil {
		t.Fatal(err)
	}

	io, out, _ := newIO("", false)
	code := Run(t.Context(), f.App, io, []string{"clean", "--yes"})
	if code != ExitOK {
		t.Fatalf("exit = %d, output:\n%s", code, out.String())
	}
	if _, err := os.Stat(filepath.Join(downloads, "customer-file.txt")); !os.IsNotExist(err) {
		t.Error("the approved demonstration data must be removed")
	}
	if _, err := os.Stat(filepath.Join(desktop, "keep-me.txt")); err != nil {
		t.Errorf("files outside the approved scope must survive: %v", err)
	}
	if _, err := os.Stat(f.Store.Dir); !os.IsNotExist(err) {
		t.Error("the WinPrep data directory must be removed")
	}
	if !f.Sysprep.Called {
		t.Error("the confirmed handover must invoke Sysprep")
	}
}

// Test C (mandatory regression): a dry run must be strictly read-only.
func TestCleanDryRunLeavesEveryFileInPlace(t *testing.T) {
	f := apptest.New(t.TempDir())
	downloads := f.DemoProfile + string(os.PathSeparator) + "Downloads"
	demoFile := filepath.Join(downloads, "demo.txt")
	if err := os.WriteFile(demoFile, []byte("demo"), 0o644); err != nil {
		t.Fatal(err)
	}

	io, out, _ := newIO("", false)
	code := Run(t.Context(), f.App, io, []string{"clean", "--dry-run"})
	if code != ExitOK {
		t.Fatalf("exit = %d", code)
	}
	if data, err := os.ReadFile(demoFile); err != nil || string(data) != "demo" {
		t.Errorf("dry run must not remove anything, got %q, %v", data, err)
	}
	if _, err := os.Stat(f.Store.Dir); err != nil {
		t.Error("dry run must keep the WinPrep data directory")
	}
	if f.Sysprep.Called {
		t.Error("dry run must never invoke Sysprep")
	}
	if !strings.Contains(out.String(), demoFile) == false {
		// The plan should have counted the file without touching it.
		if !strings.Contains(out.String(), "Downloads") || !strings.Contains(out.String(), "1 items") {
			t.Error("the dry run should preview what would be removed")
		}
	}
}
