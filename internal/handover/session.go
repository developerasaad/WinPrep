package handover

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"sort"

	"github.com/developerasaad/winprep/internal/audit"
)

// Session is one Clean & Finish attempt. Begin inspects the machine and
// builds the cleanup plan (all read-only); Approve records the operator's
// decision in memory; Execute revalidates the machine, performs the approved
// cleanup, and reseals. Nothing is modified before Approve, and Execute
// refuses to run without it, so the presentation layer cannot trigger a
// deletion by itself.
type Session struct {
	c *Controller

	// Preflight is the evaluated inspection. A Blocked verdict means no plan
	// was constructed; there is nothing to plan on a machine that fails its
	// safety checks.
	Preflight PreflightResult

	// Plan is the cleanup proposal, nil when the preflight verdict is Blocked
	// or plan construction was impossible. The plan the operator approved is
	// the plan Execute runs; a scope that changed in the meantime is refused
	// rather than silently expanded.
	Plan *CleanupPlan

	approved bool
	// resealInitiated records that Execute ended with the reseal under way,
	// which keeps the operation lock held until the process exits (the
	// machine is shutting down at that point).
	resealInitiated bool
}

// Begin inspects the machine and constructs the cleanup plan. It changes
// nothing. An error means the inspection itself failed, not that the machine
// is unsuitable; unsuitability is expressed as a Blocked verdict.
func (c *Controller) Begin(ctx context.Context) (*Session, error) {
	s := &Session{c: c}

	result, err := c.Inspect(ctx)
	if err != nil {
		return nil, err
	}
	s.Preflight = result

	if result.Verdict == VerdictBlocked {
		return s, nil
	}

	facts, err := c.Gather(ctx)
	if err != nil {
		return nil, err
	}
	plan, perr := c.BuildCleanupPlan(facts, selfPath())
	if perr != nil {
		// A plan that cannot be constructed is a blocked handover with an
		// explanation, not an inspection crash.
		s.Preflight = PreflightResult{
			Verdict: VerdictBlocked,
			Checks: append(s.Preflight.Checks, Check{
				ID:       "cleanup-plan",
				Title:    "Cleanup plan",
				Severity: Blocker,
				Detail:   perr.Error(),
				NextStep: "Nothing was changed. Resolve the issue above, then run \"Clean & Finish\" again.",
			}),
		}
		return s, nil
	}
	s.Plan = &plan
	if plan.IsBlocked() {
		s.Preflight = PreflightResult{
			Verdict: VerdictBlocked,
			Checks: append(s.Preflight.Checks, Check{
				ID:       "cleanup-plan",
				Title:    "Cleanup plan",
				Severity: Blocker,
				Detail:   "The cleanup cannot proceed: " + firstLine(plan.Blockers),
				NextStep: "Resolve the items the plan reports as blocked, then run \"Clean & Finish\" again.",
			}),
		}
	}
	return s, nil
}

// Approved reports whether the operator confirmed this session's plan.
func (s *Session) Approved() bool { return s.approved }

// Approve records the operator's confirmation. It changes nothing on disk;
// it only unlocks Execute. The confirmation must be given to exactly the
// plan held by this session.
func (s *Session) Approve() {
	s.approved = true
}

// Phase is the exact outcome of a Clean & Finish run, per the required
// outcome reporting: preflight blocked, operator cancelled, cleanup failed,
// Sysprep could not start, Sysprep reported an error, reseal initiated.
type Phase int

const (
	// PhaseCancelled means the session was never approved.
	PhaseCancelled Phase = iota
	// PhaseConditionsChanged means revalidation found the machine no longer
	// in the approved state; nothing was modified.
	PhaseConditionsChanged
	// PhaseBusy means another WinPrep process holds the operation lock.
	PhaseBusy
	// PhaseCleanupFailed means cleanup ran but did not complete; the reseal
	// was not started.
	PhaseCleanupFailed
	// PhaseResealLaunchFailed means Sysprep could not be started.
	PhaseResealLaunchFailed
	// PhaseResealSysprepError means Sysprep ran and reported an error.
	PhaseResealSysprepError
	// PhaseResealInitiated means Sysprep finished and the shutdown it
	// requested is under way. This is not a verified OOBE boot.
	PhaseResealInitiated
)

func (p Phase) String() string {
	switch p {
	case PhaseCancelled:
		return "Cancelled"
	case PhaseConditionsChanged:
		return "Conditions changed - nothing was modified"
	case PhaseBusy:
		return "Another WinPrep operation is in progress"
	case PhaseCleanupFailed:
		return "The cleanup did not complete"
	case PhaseResealLaunchFailed:
		return "Sysprep could not start"
	case PhaseResealSysprepError:
		return "Sysprep reported an error"
	case PhaseResealInitiated:
		return "Reseal initiated"
	default:
		return "Unknown"
	}
}

// Result reports the outcome of an executed session.
type Result struct {
	Phase   Phase
	Detail  string
	Cleanup ExecutionResult
	Reseal  ResealResult
}

// Succeeded reports whether the reseal was initiated. It is deliberately a
// narrow claim: an initiated reseal is not a verified OOBE boot.
func (r Result) Succeeded() bool { return r.Phase == PhaseResealInitiated }

// SummaryLines renders the result for the CLI, the fallback menu, and the
// TUI, so all three report outcomes identically.
func (r Result) SummaryLines() []string {
	var lines []string
	lines = append(lines, r.Phase.String())
	if r.Detail != "" {
		lines = append(lines, r.Detail)
	}
	if r.Phase == PhaseCleanupFailed || r.Phase == PhaseResealLaunchFailed ||
		r.Phase == PhaseResealSysprepError || r.Phase == PhaseResealInitiated {
		lines = append(lines, "")
		lines = append(lines, r.Cleanup.SummaryLines()...)
	}
	switch r.Phase {
	case PhaseResealLaunchFailed, PhaseResealSysprepError:
		if r.Reseal.Detail != "" {
			lines = append(lines, "", r.Reseal.Detail)
		}
		lines = append(lines, "", "Sysprep writes details to these logs:")
		for _, p := range PantherLogPaths() {
			lines = append(lines, "  "+p)
		}
		lines = append(lines, "",
			"Microsoft's Sysprep troubleshooting guide:",
			"https://learn.microsoft.com/en-us/troubleshoot/windows-client/setup-upgrade-and-drivers/sysprep-fails-remove-or-update-store-apps")
	case PhaseResealInitiated:
		if r.Reseal.Detail != "" {
			lines = append(lines, "", r.Reseal.Detail)
		}
		lines = append(lines, "",
			"WinPrep's part is done. The claims of a successful handover still need your",
			"own verification: switch the laptop on once and confirm that Windows",
			"first-time setup appears before selling it.")
	}
	return lines
}

// Execute runs the approved sequence: revalidate the machine, perform the
// approved cleanup, verify the results, and reseal. It must be called after
// Approve; a session that was never approved returns PhaseCancelled with the
// machine untouched. An error return means WinPrep itself failed (it could
// not re-read the machine state or acquire the operation lock); workflow
// outcomes - including every refusal - are values of Result.
//
// The operation lock is released again whenever the run ends without a
// reseal, so the operator can retry from the same window after fixing what
// failed; once the reseal is initiated the machine is shutting down and the
// lock stays held.
func (s *Session) Execute(ctx context.Context) (Result, error) {
	if !s.approved {
		return Result{Phase: PhaseCancelled,
			Detail: "The cleanup was not confirmed, so nothing was changed."}, nil
	}
	if s.Preflight.Verdict == VerdictBlocked || s.Plan == nil {
		return Result{Phase: PhaseCancelled,
			Detail: "This session is blocked, so nothing was changed."}, nil
	}

	locked := false
	if s.c.Lock != nil {
		ok, err := s.c.Lock.TryLock()
		if err != nil {
			return Result{}, fmt.Errorf("acquiring the operation lock: %w", err)
		}
		if !ok {
			return Result{Phase: PhaseBusy,
				Detail: "Another WinPrep operation is in progress on this machine. Let it finish, then try again."}, nil
		}
		locked = true
	}
	// Release the lock on every path that does not end in an initiated
	// reseal, so a retry in the same session is possible. Unlock failures
	// are not worth failing the report over; the kernel reclaims the mutex
	// when the process exits either way.
	defer func() {
		if locked && !s.resealInitiated && s.c.Lock != nil {
			_ = s.c.Lock.Unlock()
		}
	}()

	// While the approved operation runs, an interrupt must not masquerade as
	// a clean cancellation: a deletion interrupted midway must still produce
	// a report, and Sysprep must never be killed once started. Registering
	// our own channel (never read) suppresses the default exit behavior for
	// the duration; signal.Stop removes exactly this registration, leaving
	// any other component's handler untouched.
	interrupted := make(chan os.Signal, 1)
	signal.Notify(interrupted, os.Interrupt)
	defer signal.Stop(interrupted)

	// Revalidation: gather fresh facts and refuse to act on stale ones.
	facts, err := s.c.Gather(ctx)
	if err != nil {
		return Result{Phase: PhaseConditionsChanged,
			Detail: "The machine state could not be re-read, so nothing was modified. " + err.Error()}, nil
	}
	if facts.Audit.State != audit.StateAuditMode {
		return Result{Phase: PhaseConditionsChanged,
			Detail: "Windows is no longer in the confirmed Audit Mode state, so nothing was modified. " +
				"Run \"Clean & Finish\" again to inspect the machine."}, nil
	}
	if !facts.User.IsElevated {
		return Result{Phase: PhaseConditionsChanged,
			Detail: "WinPrep is no longer running with administrative privileges, so nothing was modified."}, nil
	}
	if facts.ProcsErr != nil {
		// The process list is the only way to guarantee no Sysprep process
		// is alive; an unreadable list means the overlap guarantee cannot
		// be given, so the run stops here. Failing closed.
		return Result{Phase: PhaseConditionsChanged,
			Detail: "The running-process list could not be read, so WinPrep cannot guarantee that no " +
				"Sysprep operation is active. Nothing was modified. " + facts.ProcsErr.Error()}, nil
	}
	if facts.SysprepRunning {
		return Result{Phase: PhaseConditionsChanged,
			Detail: "The System Preparation Tool window is still open. Close it, then run \"Clean & Finish\" again; " +
				"WinPrep never launches Sysprep while another Sysprep process exists."}, nil
	}

	// The full preflight evaluation is re-run on the fresh facts. Attention
	// items were reviewed when the operator confirmed, but a new blocker -
	// an account that appeared, a pending servicing restart, an active
	// Sysprep log burst - must stop the handover before anything is deleted.
	if verdict := EvaluateVerdict(BuildChecks(facts)); verdict == VerdictBlocked {
		return Result{Phase: PhaseConditionsChanged,
			Detail: "A condition that blocks the handover appeared since the plan was approved, so nothing " +
				"was modified. Run \"Clean & Finish\" again to see the updated inspection."}, nil
	}

	replan, err := s.c.BuildCleanupPlan(facts, selfPath())
	if err != nil {
		return Result{Phase: PhaseConditionsChanged,
			Detail: "The cleanup plan could not be revalidated, so nothing was modified. " + err.Error()}, nil
	}
	if replan.IsBlocked() {
		return Result{Phase: PhaseConditionsChanged,
			Detail: "The machine no longer matches the approved plan: " + firstLine(replan.Blockers) +
				" Nothing was modified; run \"Clean & Finish\" again to review the new plan."}, nil
	}
	if !sameScope(replan, *s.Plan) {
		return Result{Phase: PhaseConditionsChanged,
			Detail: "The cleanup scope changed since the plan was approved, so nothing was modified. " +
				"Run \"Clean & Finish\" again to review the new plan."}, nil
	}

	// The approved cleanup. The operator confirmed a set of locations, not a
	// frozen file list: the revalidated plan operates on the same roots, and
	// files that appeared inside an approved location since the confirmation
	// are covered by that confirmation.
	cleanup := ExecutePlan(replan)
	if !cleanup.Complete {
		return Result{Phase: PhaseCleanupFailed, Cleanup: cleanup,
			Detail: "The reseal was NOT started because the cleanup did not complete."}, nil
	}

	reseal, rerr := s.c.Reseal(ctx)
	if rerr != nil {
		return Result{Phase: PhaseResealLaunchFailed, Cleanup: cleanup,
			Detail: "Starting Sysprep failed: " + rerr.Error()}, nil
	}
	switch reseal.Outcome {
	case ResealInitiated:
		s.resealInitiated = true
		return Result{Phase: PhaseResealInitiated, Cleanup: cleanup, Reseal: reseal}, nil
	case ResealSysprepError:
		return Result{Phase: PhaseResealSysprepError, Cleanup: cleanup, Reseal: reseal}, nil
	default:
		return Result{Phase: PhaseResealLaunchFailed, Cleanup: cleanup, Reseal: reseal}, nil
	}
}

// ConfirmationWarning is the one canonical confirmation text, shown by the
// CLI, the TUI, and the fallback menu so every interface warns identically.
func ConfirmationWarning() string {
	return `WinPrep will now:
  - Remove the approved demonstration data listed in the plan above.
  - Reseal Windows into first-time setup (OOBE). The existing Windows
    installation, its drivers, and its programs are kept; Windows is not
    reinstalled.
  - Shut the laptop down when Sysprep finishes.

Please note:
  - Sysprep is not a secure erase. Data outside the approved cleanup scope
    remains on the disk. Wipe the disk separately if it must be gone.
  - After the handover, start the laptop once yourself and verify that
    Windows first-time setup appears, before handing it to the buyer.`
}

// selfPath is the running executable's path, or empty when unknown.
func selfPath() string {
	p, err := os.Executable()
	if err != nil {
		return ""
	}
	return p
}

// sameScope reports whether two plans act on the same set of locations. The
// operator approved a specific scope; files coming and going inside an
// approved location are expected, but a different set of locations is a
// different plan that needs a new confirmation.
func sameScope(a, b CleanupPlan) bool {
	ra, rb := sameRoots(a), sameRoots(b)
	if len(ra) != len(rb) {
		return false
	}
	for i := range ra {
		if ra[i] != rb[i] {
			return false
		}
	}
	return true
}

func sameRoots(p CleanupPlan) []string {
	var roots []string
	for _, group := range [][]PlanTarget{p.WinPrepTargets, p.DemoTargets} {
		for _, t := range group {
			roots = append(roots, normalizePath(t.Root))
		}
	}
	sort.Strings(roots)
	return roots
}

func firstLine(lines []string) string {
	if len(lines) == 0 {
		return ""
	}
	return lines[0]
}
