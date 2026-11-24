//go:build windows

package windows

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"syscall"
)

// CommandRunner implements platform.Runner with os/exec. Arguments are passed
// as an argument vector; no shell is ever involved. The context is honored:
// a cancelled context kills the child process. The reseal deliberately passes
// an uncancellable context, so in practice nothing launched here is ever
// interrupted mid-run.
type CommandRunner struct{}

// NewRunner returns the Windows command runner.
func NewRunner() *CommandRunner { return &CommandRunner{} }

// RunVisible implements platform.Runner. The command is allowed to show its
// own windows and interact with the user, which is required for Sysprep. A
// non-zero exit status is a legitimate, reportable result and is returned as
// the exit code, not as an error.
func (r *CommandRunner) RunVisible(ctx context.Context, name string, args ...string) (int, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: false}
	err := cmd.Run()
	if err == nil {
		return 0, nil
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return ee.ExitCode(), nil
	}
	return 0, fmt.Errorf("running %s: %w", name, err)
}
