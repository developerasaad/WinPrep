package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/developerasaad/winprep/internal/apptest"
)

// driveFallback feeds the given input lines to the fallback menu and returns
// everything it printed plus the exit code.
func driveFallback(t *testing.T, f *apptest.Fixture, input string) (string, int) {
	t.Helper()
	var out strings.Builder
	code := RunFallback(t.Context(), f.App, strings.NewReader(input), &out)
	return out.String(), code
}

func TestFallbackExitImmediately(t *testing.T) {
	f := apptest.New(t.TempDir())
	out, code := driveFallback(t, f, "5\n")
	if code != 0 {
		t.Errorf("exit = %d, want 0", code)
	}
	if !strings.Contains(out, "1  Prepare for display") {
		t.Error("the fallback menu must list its options")
	}
}

func TestFallbackInvalidThenValid(t *testing.T) {
	f := apptest.New(t.TempDir())
	out, code := driveFallback(t, f, "9\nx\n5\n")
	if code != 0 {
		t.Errorf("exit = %d, want 0", code)
	}
	if !strings.Contains(out, "Please enter a number from 1 to 5.") {
		t.Error("invalid input must be rejected with a clear message")
	}
	if strings.Count(out, "Please enter a number from 1 to 5.") != 2 {
		t.Errorf("two invalid entries expected, output:\n%s", out)
	}
}

func TestFallbackStatusShowsReport(t *testing.T) {
	f := apptest.New(t.TempDir())
	out, code := driveFallback(t, f, "4\n\n5\n")
	if code != 0 {
		t.Errorf("exit = %d, want 0", code)
	}
	for _, want := range []string{"Windows 11 Pro", "Audit Mode (confirmed)", "Press ENTER to return to the menu."} {
		if !strings.Contains(out, want) {
			t.Errorf("fallback status output missing %q", want)
		}
	}
}

func TestFallbackHandoverDeclined(t *testing.T) {
	f := apptest.New(t.TempDir())
	out, code := driveFallback(t, f, "3\nn\n5\n")
	if code != 0 {
		t.Errorf("exit = %d, want 0", code)
	}
	if !strings.Contains(out, "Handover inspection") {
		t.Error("the inspection must be shown")
	}
	if !strings.Contains(out, "Cancelled. Nothing further was changed") {
		t.Error("a declined handover must be reported as cancelled")
	}
	if f.Sysprep.Called {
		t.Error("a declined handover must not start Sysprep")
	}
}

func TestFallbackHandoverConfirmed(t *testing.T) {
	f := apptest.New(t.TempDir())
	out, code := driveFallback(t, f, "3\ny\n5\n")
	if code != 0 {
		t.Errorf("exit = %d, want 0", code)
	}
	if !strings.Contains(out, "Reseal initiated") {
		t.Error("a confirmed handover should report the initiated reseal")
	}
	if !strings.Contains(out, "Sysprep does NOT securely erase data") {
		t.Error("the confirmation must include the honest warnings")
	}
	if !f.Sysprep.Called {
		t.Error("a confirmed handover must invoke Sysprep")
	}
}

func TestFallbackHandoverBlocked(t *testing.T) {
	f := apptest.New(t.TempDir())
	f.Sys.Usr.IsElevated = false
	out, code := driveFallback(t, f, "3\n5\n")
	if code != 0 {
		t.Errorf("exit = %d, want 0", code)
	}
	if !strings.Contains(out, "The handover cannot proceed") {
		t.Error("a blocked handover must be refused")
	}
	if f.Sysprep.Called {
		t.Error("a blocked handover must not start Sysprep")
	}
}

func TestFallbackCheckExport(t *testing.T) {
	f := apptest.New(t.TempDir())
	dir := t.TempDir()
	path := filepath.Join(dir, "report.txt")
	out, code := driveFallback(t, f, "2\ny\n"+path+"\n5\n")
	if code != 0 {
		t.Errorf("exit = %d, want 0", code)
	}
	if !strings.Contains(out, "WinPrep diagnostic report") {
		t.Error("the diagnostics report must be printed")
	}
	if !strings.Contains(out, "Report saved to: "+path) {
		t.Errorf("the export must be confirmed, output:\n%s", out)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("the report file should exist: %v", err)
	}
}

func TestFallbackEofEndsQuietly(t *testing.T) {
	f := apptest.New(t.TempDir())
	out, code := driveFallback(t, f, "")
	if code != 0 {
		t.Errorf("exit = %d, want 0", code)
	}
	if !strings.Contains(out, "Choose 1-5") {
		t.Error("the menu should have been shown once")
	}
}
