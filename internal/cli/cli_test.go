package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/developerasaad/winprep/internal/apptest"
	"github.com/developerasaad/winprep/internal/platform"
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

func TestStatusShowsSystemAndAuditState(t *testing.T) {
	f := apptest.New(t.TempDir())
	io, out, _ := newIO("", false)
	if code := Run(t.Context(), f.App, io, []string{"status"}); code != ExitOK {
		t.Fatalf("exit = %d", code)
	}
	s := out.String()
	for _, want := range []string{
		"Windows 11 Pro",
		"Audit Mode (confirmed)",
		"Suitable for the WinPrep workflow: Yes",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("status output missing %q", want)
		}
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

func TestCheckRunsDiagnostics(t *testing.T) {
	f := apptest.New(t.TempDir())
	io, out, _ := newIO("", false)
	if code := Run(t.Context(), f.App, io, []string{"check"}); code != ExitOK {
		t.Fatalf("exit = %d", code)
	}
	s := out.String()
	for _, want := range []string{"Computer", "Storage", "Battery", "Devices with problems", "Windows activation"} {
		if !strings.Contains(s, want) {
			t.Errorf("check output missing section %q", want)
		}
	}
	if !f.Run.Invoked("powershell.exe") {
		t.Error("the hardware query should have run through powershell")
	}
}

func TestCheckExportWritesCRLFFile(t *testing.T) {
	f := apptest.New(t.TempDir())
	dir := t.TempDir()
	path := filepath.Join(dir, "report.txt")

	io, out, _ := newIO("", false)
	code := Run(t.Context(), f.App, io, []string{"check", "--export", path})
	if code != ExitOK {
		t.Fatalf("exit = %d, output:\n%s", code, out.String())
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(data, []byte("\r\n")) {
		t.Error("exported report should use Windows line endings")
	}
	if !strings.Contains(string(data), "WinPrep diagnostic report") {
		t.Error("exported report is missing its header")
	}
	if !strings.Contains(out.String(), "Report saved to:") {
		t.Error("CLI should confirm where the report was saved")
	}
}

func TestCheckBatteryReport(t *testing.T) {
	f := apptest.New(t.TempDir())
	dir := t.TempDir()
	path := filepath.Join(dir, "battery.html")
	// The fake powercfg succeeds without writing anything, so the file is
	// created beforehand; the service verifies its presence afterwards.
	if err := os.WriteFile(path, []byte("<html></html>"), 0o644); err != nil {
		t.Fatal(err)
	}

	io, out, _ := newIO("", false)
	code := Run(t.Context(), f.App, io, []string{"check", "--battery-report", path})
	if code != ExitOK {
		t.Fatalf("exit = %d, output:\n%s", code, out.String())
	}
	if !strings.Contains(out.String(), "Battery report saved to:") {
		t.Error("CLI should confirm the battery report location")
	}
	if !f.Run.Invoked("powercfg.exe") {
		t.Error("powercfg should have been invoked")
	}
}

func TestCheckRejectsMissingExportPath(t *testing.T) {
	f := apptest.New(t.TempDir())
	io, _, errOut := newIO("", false)
	code := Run(t.Context(), f.App, io, []string{"check", "--export"})
	if code != ExitUsage {
		t.Errorf("exit = %d, want %d", code, ExitUsage)
	}
	if !strings.Contains(errOut.String(), "--export needs a file path") {
		t.Errorf("stderr = %q", errOut.String())
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
	if len(f.Run.Commands) == 0 {
		t.Error("the inspection should still have run")
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
	f.Sysprep.Res = platform.CommandResult{ExitCode: 1, Stderr: "sysexe error"}
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
