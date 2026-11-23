//go:build windows

package windows

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"syscall"

	"github.com/developerasaad/winprep/internal/platform"
)

// CommandRunner implements platform.Runner with os/exec. Arguments are passed
// as an argument vector; no shell is ever involved.
type CommandRunner struct{}

// NewRunner returns the Windows command runner.
func NewRunner() *CommandRunner { return &CommandRunner{} }

// run executes a command and captures its output. The context is honored: a
// cancelled context kills the child process, which is how the TUI cancels
// long read-only queries (the hardware inspection) without leaving orphaned
// powershell.exe processes behind.
func (r *CommandRunner) run(ctx context.Context, hidden bool, name string, args ...string) (platform.CommandResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	cmd := exec.CommandContext(ctx, name, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: hidden}
	err := cmd.Run()

	res := platform.CommandResult{
		Stdout: stdout.String(),
		Stderr: stderr.String(),
	}
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			res.ExitCode = ee.ExitCode()
			// A non-zero exit status is a legitimate, reportable result, not
			// a failure to run.
			return res, nil
		}
		return res, fmt.Errorf("running %s: %w", name, err)
	}
	return res, nil
}

// Run implements platform.Runner.
func (r *CommandRunner) Run(ctx context.Context, name string, args ...string) (platform.CommandResult, error) {
	return r.run(ctx, true, name, args...)
}

// RunVisible implements platform.Runner.
func (r *CommandRunner) RunVisible(ctx context.Context, name string, args ...string) (platform.CommandResult, error) {
	return r.run(ctx, false, name, args...)
}

// Cancelled reports whether a command result ended because its context was
// cancelled, which the caller treats as "operation cancelled" rather than
// "operation failed". Kept as a helper so all callers agree on the check.
func Cancelled(err error) bool {
	return err != nil && strings.Contains(err.Error(), "context canceled")
}
