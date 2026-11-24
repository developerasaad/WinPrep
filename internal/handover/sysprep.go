package handover

import (
	"context"
	"fmt"
)

// SysprepRunner abstracts the single privileged call WinPrep makes: invoking
// Sysprep to reseal the machine. The Windows implementation runs
// %WINDIR%\System32\Sysprep\Sysprep.exe /oobe /shutdown.
type SysprepRunner interface {
	// SysprepPath returns the absolute path of Sysprep.exe, or an error if it
	// cannot be located.
	SysprepPath() (string, error)
	// RunReseal runs the documented reseal command and returns the Sysprep
	// exit code. Implementations must run it with a non-cancellable
	// context: once Sysprep has accepted the operation it must not be
	// killed by WinPrep. err is non-nil only when Sysprep could not be
	// started or waited for.
	RunReseal(ctx context.Context) (exitCode int, err error)
}

// ResealOutcome distinguishes exactly what happened, per the seven-step
// procedure. "Initiated" is not "completed": actual OOBE behavior can only be
// confirmed by booting the machine, and WinPrep does not pretend otherwise.
type ResealOutcome int

const (
	// ResealNotStarted means the reseal never ran.
	ResealNotStarted ResealOutcome = iota
	// ResealLaunchFailed means Sysprep could not be started at all.
	ResealLaunchFailed
	// ResealSysprepError means Sysprep ran and reported an error.
	ResealSysprepError
	// ResealInitiated means Sysprep ran to completion and the shutdown it
	// requested has been set in motion.
	ResealInitiated
)

func (o ResealOutcome) String() string {
	switch o {
	case ResealLaunchFailed:
		return "Sysprep could not start"
	case ResealSysprepError:
		return "Sysprep reported an error"
	case ResealInitiated:
		return "Reseal initiated"
	default:
		return "Not started"
	}
}

// ResealResult reports the outcome of the reseal attempt.
type ResealResult struct {
	Outcome  ResealOutcome
	ExitCode int
	Detail   string
}

// PantherLogPaths lists the Sysprep log locations Windows writes when the
// reseal succeeds or fails. They are shown with every non-happy outcome and
// preserved untouched.
func PantherLogPaths() []string {
	return []string{
		`%WINDIR%\System32\Sysprep\Panther\setupact.log`,
		`%WINDIR%\System32\Sysprep\Panther\setuperr.log`,
		`%WINDIR%\Panther\setupact.log`,
		`%WINDIR%\Panther\setuperr.log`,
	}
}

// Reseal performs the final handover step. It must only be called after the
// preflight passed and the operator confirmed. The context passed in is used
// only for pre-launch setup; the Sysprep invocation itself is deliberately
// not cancellable, because killing Sysprep mid-run can leave Windows in a
// broken deployment state.
func (c *Controller) Reseal(ctx context.Context) (ResealResult, error) {
	path, err := c.Sysprep.SysprepPath()
	if err != nil {
		return ResealResult{
			Outcome: ResealLaunchFailed,
			Detail:  fmt.Sprintf("Sysprep could not be found: %v. Check that Windows is installed completely.", err),
		}, nil
	}

	exitCode, err := c.Sysprep.RunReseal(context.WithoutCancel(ctx))
	if err != nil {
		return ResealResult{
			Outcome: ResealLaunchFailed,
			Detail:  fmt.Sprintf("Starting %s failed: %v", path, err),
		}, nil
	}

	switch {
	case exitCode == 0:
		return ResealResult{
			Outcome: ResealInitiated,
			Detail: "Sysprep finished and initiated the shutdown. When the laptop is next switched on, it will start " +
				"Windows first-time setup. Verify this before selling the laptop.",
		}, nil
	default:
		return ResealResult{
			Outcome:  ResealSysprepError,
			ExitCode: exitCode,
			Detail: fmt.Sprintf("Sysprep exited with code %d and the machine was NOT prepared for handover. "+
				"Windows may show a Sysprep error dialog; dismiss it, then check the logs below.", exitCode),
		}, nil
	}
}
