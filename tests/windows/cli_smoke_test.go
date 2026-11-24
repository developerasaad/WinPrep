//go:build windows

package windows

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// winprepExe locates the executable under test. CI points WINPREP_EXE at the
// binary it built; locally the test builds one so `go test ./...` on a
// Windows development machine covers the smoke tests too.
func winprepExe(t *testing.T) string {
	t.Helper()
	if p := os.Getenv("WINPREP_EXE"); p != "" {
		if _, err := os.Stat(p); err != nil {
			t.Fatalf("WINPREP_EXE=%s does not exist: %v", p, err)
		}
		return p
	}
	p := filepath.Join(t.TempDir(), "winprep.exe")
	cmd := exec.Command("go", "build", "-o", p, "github.com/developerasaad/winprep/cmd/winprep")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("building winprep.exe: %v\n%s", err, out)
	}
	return p
}

// run executes the exe and returns combined output plus the exit code.
func run(t *testing.T, exe string, stdin *strings.Reader, args ...string) (string, int) {
	t.Helper()
	cmd := exec.Command(exe, args...)
	var out strings.Builder
	cmd.Stdout = &out
	cmd.Stderr = &out
	if stdin != nil {
		cmd.Stdin = stdin
	}
	err := cmd.Run()
	code := 0
	if ee, ok := err.(*exec.ExitError); ok {
		code = ee.ExitCode()
	} else if err != nil {
		t.Fatalf("running %s %v: %v", exe, args, err)
	}
	return out.String(), code
}

func TestVersionCommand(t *testing.T) {
	exe := winprepExe(t)
	out, code := run(t, exe, nil, "version")
	if code != 0 {
		t.Errorf("exit = %d, output:\n%s", code, out)
	}
	if !strings.HasPrefix(strings.TrimSpace(out), "winprep ") {
		t.Errorf("version output should start with the tool name:\n%s", out)
	}
	if !strings.Contains(out, "windows/amd64") {
		t.Errorf("version output should name windows/amd64:\n%s", out)
	}
}

func TestHelpCommand(t *testing.T) {
	exe := winprepExe(t)
	out, code := run(t, exe, nil, "help")
	if code != 0 {
		t.Errorf("exit = %d", code)
	}
	for _, want := range []string{"winprep persist", "winprep clean --dry-run", "Exit codes"} {
		if !strings.Contains(out, want) {
			t.Errorf("help output missing %q", want)
		}
	}
}

func TestUnknownCommandExitsWithUsageCode(t *testing.T) {
	exe := winprepExe(t)
	out, code := run(t, exe, nil, "definitely-not-a-command")
	if code != 2 {
		t.Errorf("exit = %d, want 2", code)
	}
	if !strings.Contains(out, "Unknown command") {
		t.Errorf("output should name the unknown command:\n%s", out)
	}
}

func TestCleanDryRunOnDeployedMachineIsRefused(t *testing.T) {
	exe := winprepExe(t)
	out, code := run(t, exe, nil, "clean", "--dry-run")
	if code != 0 {
		t.Fatalf("dry run is an inspection and exits 0 even when blocked; exit = %d, output:\n%s", code, out)
	}
	if !strings.Contains(out, "Dry run: no changes were made") {
		t.Errorf("dry run must state that nothing changed:\n%s", out)
	}
	// The CI runner is a normally deployed Windows installation (and Server,
	// on GitHub-hosted runners), so the handover must be refused - never
	// attempted - with the reason shown.
	if !strings.Contains(out, "NOT possible") {
		t.Errorf("a deployed machine must not pass the handover preflight:\n%s", out)
	}
	if strings.Contains(out, "Starting Sysprep") {
		t.Error("dry run must never start Sysprep")
	}
}

func TestCleanWithoutConfirmationDoesNothing(t *testing.T) {
	exe := winprepExe(t)
	// The machine is not in Audit Mode, so the blocked verdict stops the
	// procedure before any prompt or confirmation shortcut matters.
	out, code := run(t, exe, nil, "clean", "--yes")
	if code != 1 {
		t.Errorf("exit = %d, want 1 for a blocked handover", code)
	}
	if !strings.Contains(out, "cannot proceed") {
		t.Errorf("output should say the handover cannot proceed:\n%s", out)
	}
	if strings.Contains(out, "Starting Sysprep") {
		t.Error("a blocked machine must never start Sysprep, even with --yes")
	}
}

func TestExecutableRunsFromPathWithSpaces(t *testing.T) {
	src := winprepExe(t)
	dir := t.TempDir()
	spaced := filepath.Join(dir, "a path with spaces")
	if err := os.MkdirAll(spaced, 0o755); err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(spaced, "winprep.exe")
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, data, 0o755); err != nil {
		t.Fatal(err)
	}
	out, code := run(t, dst, nil, "version")
	if code != 0 {
		t.Errorf("exit = %d running from %s, output:\n%s", code, spaced, out)
	}
}

func TestNoTerminalGivesGuidanceNotABrokenMenu(t *testing.T) {
	exe := winprepExe(t)
	// stdin/stdout are pipes here, not a console: the interactive menu must
	// decline politely and point at the CLI.
	out, code := run(t, exe, strings.NewReader(""))
	if code != 2 {
		t.Errorf("exit = %d, want 2 without a terminal", code)
	}
	if !strings.Contains(out, "needs an interactive terminal") {
		t.Errorf("output should explain the missing terminal:\n%s", out)
	}
	if !strings.Contains(out, "winprep help") {
		t.Errorf("output should point at the CLI help:\n%s", out)
	}
}
