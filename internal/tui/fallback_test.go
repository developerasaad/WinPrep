package tui

import (
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
	out, code := driveFallback(t, f, "4\n")
	if code != 0 {
		t.Errorf("exit = %d, want 0", code)
	}
	if !strings.Contains(out, "1  Prepare for Display") {
		t.Error("the fallback menu must list its options")
	}
	if !strings.Contains(out, "2  Clean & Finish") {
		t.Error("the fallback menu must list the handover option")
	}
}

func TestFallbackInvalidThenValid(t *testing.T) {
	f := apptest.New(t.TempDir())
	out, code := driveFallback(t, f, "9\nx\n4\n")
	if code != 0 {
		t.Errorf("exit = %d, want 0", code)
	}
	if !strings.Contains(out, "Please enter a number from 1 to 4.") {
		t.Error("invalid input must be rejected with a clear message")
	}
	if strings.Count(out, "Please enter a number from 1 to 4.") != 2 {
		t.Errorf("two invalid entries expected, output:\n%s", out)
	}
}

func TestFallbackHelpShowsText(t *testing.T) {
	f := apptest.New(t.TempDir())
	out, code := driveFallback(t, f, "3\n\n4\n")
	if code != 0 {
		t.Errorf("exit = %d, want 0", code)
	}
	for _, want := range []string{"The shop workflow:", "What WinPrep does NOT do:", "Press ENTER to return to the menu."} {
		if !strings.Contains(out, want) {
			t.Errorf("fallback help output missing %q", want)
		}
	}
}

func TestFallbackHandoverDeclined(t *testing.T) {
	f := apptest.New(t.TempDir())
	out, code := driveFallback(t, f, "2\nn\n4\n")
	if code != 0 {
		t.Errorf("exit = %d, want 0", code)
	}
	if !strings.Contains(out, "Clean & Finish - inspection") {
		t.Error("the inspection must be shown")
	}
	if !strings.Contains(out, "Cancelled. Nothing was changed") {
		t.Error("a declined handover must be reported as cancelled")
	}
	if f.Sysprep.Called {
		t.Error("a declined handover must not start Sysprep")
	}
}

func TestFallbackHandoverConfirmed(t *testing.T) {
	f := apptest.New(t.TempDir())
	out, code := driveFallback(t, f, "2\ny\n4\n")
	if code != 0 {
		t.Errorf("exit = %d, want 0", code)
	}
	if !strings.Contains(out, "Reseal initiated") {
		t.Error("a confirmed handover should report the initiated reseal")
	}
	if !strings.Contains(out, "Sysprep is not a secure erase") {
		t.Error("the confirmation must include the honest warnings")
	}
	if !f.Sysprep.Called {
		t.Error("a confirmed handover must invoke Sysprep")
	}
}

func TestFallbackHandoverBlocked(t *testing.T) {
	f := apptest.New(t.TempDir())
	f.Sys.Usr.IsElevated = false
	out, code := driveFallback(t, f, "2\n4\n")
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

func TestFallbackEofEndsQuietly(t *testing.T) {
	f := apptest.New(t.TempDir())
	out, code := driveFallback(t, f, "")
	if code != 0 {
		t.Errorf("exit = %d, want 0", code)
	}
	if !strings.Contains(out, "Choose 1-4") {
		t.Error("the menu should have been shown once")
	}
}
