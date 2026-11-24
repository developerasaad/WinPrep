// Package handover implements the "Ready for buyer" operation: preflight
// validation of the machine, removal of WinPrep's own artifacts, and the
// Sysprep call that returns the laptop to Windows first-time setup.
//
// The preflight evaluation is deliberately pessimistic. Unknown or conflicting
// states block the automatic reseal, because WinPrep's failure mode must be
// "refuse to act", never "act on a guess".
package handover

import (
	"fmt"
	"strings"
	"time"

	"github.com/developerasaad/winprep/internal/audit"
	"github.com/developerasaad/winprep/internal/platform"
)

// Severity classifies a single preflight check.
type Severity int

const (
	// Pass means the check succeeded and nothing needs doing.
	Pass Severity = iota
	// Attention means the check found an issue the operator must review.
	// Proceeding is allowed after the operator acknowledges it.
	Attention
	// Blocker means the operation must not proceed under any circumstances
	// until the condition is resolved.
	Blocker
	// UnknownSeverity means the check could not establish its fact. It blocks
	// automatic resealing just like a blocker.
	UnknownSeverity
)

func (s Severity) String() string {
	switch s {
	case Pass:
		return "Passed"
	case Attention:
		return "Requires attention"
	case Blocker:
		return "Blocked"
	default:
		return "Unknown"
	}
}

// Verdict is the overall preflight result derived from all checks.
type Verdict int

const (
	// VerdictReady means every check passed; the handover may proceed.
	VerdictReady Verdict = iota
	// VerdictAttention means non-critical issues need operator review before
	// proceeding.
	VerdictAttention
	// VerdictBlocked means at least one blocker or unknown check exists; the
	// handover must not proceed.
	VerdictBlocked
)

func (v Verdict) String() string {
	switch v {
	case VerdictReady:
		return "Ready"
	case VerdictAttention:
		return "Requires attention"
	default:
		return "Blocked"
	}
}

// AccountFact is a local account observed during inspection.
type AccountFact struct {
	Name     string
	SID      string
	Disabled bool
}

// ProfileFact is a user profile registered on the machine.
type ProfileFact struct {
	SID  string
	Path string
}

// Facts is the raw, unevaluated state gathered before a handover. Fields with
// an Err counterpart record whether gathering that fact failed; failures turn
// into Unknown checks rather than silently missing issues.
type Facts struct {
	System platform.SystemInfo
	User   platform.UserInfo
	Audit  audit.Assessment

	Accounts       []AccountFact
	AccountsErr    error
	Profiles       []ProfileFact
	ProfilesErr    error
	OrphanDirs     []string
	SysprepPresent bool
	SysprepErr     error
	SysprepRunning bool
	ProcessNames   []string
	ProcsErr       error
	// SysprepLogTimes are the modification times of the existing Sysprep
	// logs, used to distinguish an idle System Preparation Tool window from
	// an operation in progress.
	SysprepLogTimes   []time.Time
	SysprepLogErr     error
	PendingCBSReboot  bool
	PendingWUReboot   bool
	PendingFileRename bool
	RebootErr         error
	// Now is the moment the facts were gathered, so log freshness can be
	// judged without hidden clock reads in the pure check evaluation.
	Now time.Time
}

// Check is one evaluated preflight check.
type Check struct {
	ID       string
	Title    string
	Severity Severity
	Detail   string
	NextStep string
}

// PreflightResult is the evaluated outcome shown to the operator.
type PreflightResult struct {
	Verdict Verdict
	Checks  []Check
}

// EvaluateVerdict derives the overall verdict from the checks:
//
//   - any Blocker or Unknown check -> Blocked
//   - else any Attention check      -> Attention
//   - else                          -> Ready
//
// Unknown checks block because "we could not tell" is not permission to act.
func EvaluateVerdict(checks []Check) Verdict {
	verdict := VerdictReady
	for _, c := range checks {
		switch c.Severity {
		case Blocker, UnknownSeverity:
			return VerdictBlocked
		case Attention:
			verdict = VerdictAttention
		}
	}
	return verdict
}

// BuildChecks converts gathered facts into the ordered check list. It is pure
// so the mapping can be unit tested exhaustively.
func BuildChecks(f Facts) []Check {
	var checks []Check
	add := func(c Check) { checks = append(checks, c) }

	// 1. Administrative privileges. Sysprep refuses to run without them.
	if f.User.IsElevated {
		add(Check{ID: "privileges", Title: "Administrative privileges", Severity: Pass,
			Detail: "WinPrep is running with an elevated token."})
	} else {
		add(Check{ID: "privileges", Title: "Administrative privileges", Severity: Blocker,
			Detail:   "WinPrep is not running as administrator. Sysprep requires administrative privileges.",
			NextStep: "Close this window, right-click Command Prompt, choose \"Run as administrator\", and start WinPrep again."})
	}

	// 2. Supported Windows release (Windows 10/11 client, x64).
	switch {
	case !f.System.IsWindows:
		add(Check{ID: "windows", Title: "Supported Windows system", Severity: Blocker,
			Detail:   "This is not a Windows system. WinPrep supports Windows 10 and Windows 11 only.",
			NextStep: "Run WinPrep on the Windows laptop being prepared."})
	case f.System.IsServer():
		add(Check{ID: "windows", Title: "Supported Windows system", Severity: Blocker,
			Detail:   fmt.Sprintf("This is Windows Server (%s). WinPrep supports the client editions of Windows 10 and 11 only.", f.System.OSName),
			NextStep: "Use WinPrep on a Windows 10 or 11 laptop."})
	case f.System.Architecture != "x64":
		add(Check{ID: "windows", Title: "Supported Windows system", Severity: Blocker,
			Detail:   fmt.Sprintf("This build targets %s systems. WinPrep currently ships x64 builds only.", f.System.Architecture),
			NextStep: "Use the x64 build of WinPrep."})
	case f.System.Build == "":
		add(Check{ID: "windows", Title: "Supported Windows system", Severity: UnknownSeverity,
			Detail:   "The Windows build number could not be read, so the release cannot be verified.",
			NextStep: "Restart Windows and try again; if it persists, report this as a bug."})
	default:
		add(Check{ID: "windows", Title: "Supported Windows system", Severity: Pass,
			Detail: fmt.Sprintf("%s (build %s) on %s.", f.System.OSName, f.System.Build, f.System.Architecture)})
	}

	// 3. Audit Mode state. The handover only supports the documented
	// same-device Audit Mode workflow.
	switch f.Audit.State {
	case audit.StateAuditMode:
		add(Check{ID: "audit", Title: "Audit Mode", Severity: Pass,
			Detail: f.Audit.Summary})
	case audit.StateNormalInstalled:
		add(Check{ID: "audit", Title: "Audit Mode", Severity: Blocker,
			Detail: "This Windows installation is already deployed (setup is complete). Re-running Sysprep on a " +
				"machine that has been in normal use is not the supported workflow and can fail in ways that are " +
				"hard to recover from.",
			NextStep: "Prepare laptops by reinstalling Windows, entering Audit Mode before completing first-time setup, " +
				"and running WinPrep there. See docs/workflow.md."})
	case audit.StateOOBE:
		add(Check{ID: "audit", Title: "Audit Mode", Severity: Blocker,
			Detail: "Windows first-time setup (OOBE) is currently in progress.",
			NextStep: "Press Ctrl+Shift+F3 on the setup screen to enter Audit Mode, or complete setup normally if " +
				"you intend to keep using this installation."})
	default:
		add(Check{ID: "audit", Title: "Audit Mode", Severity: UnknownSeverity,
			Detail: f.Audit.Summary,
			NextStep: "WinPrep cannot confirm Audit Mode, so it will not reseal this machine. Restart and try again; " +
				"if the state stays unclear, report this as a bug."})
	}

	// 4. Sysprep availability.
	switch {
	case f.SysprepErr != nil:
		add(Check{ID: "sysprep", Title: "Sysprep availability", Severity: UnknownSeverity,
			Detail:   "Could not verify that Sysprep is present: " + f.SysprepErr.Error(),
			NextStep: "Check that %WINDIR%\\System32\\Sysprep\\Sysprep.exe exists."})
	case f.SysprepPresent:
		add(Check{ID: "sysprep", Title: "Sysprep availability", Severity: Pass,
			Detail: "Sysprep was found in the expected location."})
	default:
		add(Check{ID: "sysprep", Title: "Sysprep availability", Severity: Blocker,
			Detail:   "Sysprep.exe is not present in %WINDIR%\\System32\\Sysprep.",
			NextStep: "This Windows installation appears incomplete. Reinstall Windows before preparing the laptop."})
	}

	// 5. Sysprep conflict. A sysprep.exe process can be the idle System
	// Preparation Tool window that Audit Mode opens at every sign-in, so the
	// process alone no longer blocks the inspection. Fresh Panther-log writes
	// mean the engine is working right now, which blocks; stale or absent
	// logs mean the window is idle, which is called out for the operator to
	// close before the final step. The execution gate later refuses to launch
	// while any sysprep.exe process is alive, so an idle window can never turn
	// into two overlapping operations.
	switch {
	case f.ProcsErr != nil:
		add(Check{ID: "sysprep-idle", Title: "No active Sysprep operation", Severity: UnknownSeverity,
			Detail:   "Could not list running processes: " + f.ProcsErr.Error(),
			NextStep: "Close any Sysprep windows and try again."})
	case !f.SysprepRunning:
		add(Check{ID: "sysprep-idle", Title: "No active Sysprep operation", Severity: Pass,
			Detail: "No Sysprep process is running."})
	case f.SysprepLogErr != nil:
		add(Check{ID: "sysprep-idle", Title: "No active Sysprep operation", Severity: UnknownSeverity,
			Detail:   "A Sysprep process is running and its activity could not be read: " + f.SysprepLogErr.Error(),
			NextStep: "Close the System Preparation Tool window and try again."})
	case sysprepActive(f):
		add(Check{ID: "sysprep-idle", Title: "No active Sysprep operation", Severity: Blocker,
			Detail:   "A Sysprep operation appears to be in progress: its logs were written seconds ago. Two Sysprep operations must never overlap.",
			NextStep: "Wait for it to finish (the machine may shut down), then run \"Clean & Finish\" again."})
	default:
		add(Check{ID: "sysprep-idle", Title: "No active Sysprep operation", Severity: Attention,
			Detail: "The System Preparation Tool window is open but idle (its logs have not been written recently).",
			NextStep: "Close the window yourself before the final step; WinPrep never closes it, because an " +
				"interrupted Sysprep operation can leave Windows in a broken state."})
	}

	// 6. Pending servicing operations. A pending Component Based Servicing or
	// Windows Update restart is a well-known cause of Sysprep failure, so it
	// blocks. Stale file-rename operations alone are common and often
	// harmless, so they only warrant attention.
	switch {
	case f.RebootErr != nil:
		add(Check{ID: "pending-reboot", Title: "No pending restart", Severity: UnknownSeverity,
			Detail:   "Could not read the pending-restart indicators: " + f.RebootErr.Error(),
			NextStep: "Restart Windows and run \"Ready for buyer\" again."})
	case f.PendingCBSReboot || f.PendingWUReboot:
		why := "Windows has a restart pending"
		if f.PendingCBSReboot && f.PendingWUReboot {
			why += " (component servicing and Windows Update)"
		} else if f.PendingCBSReboot {
			why += " (component servicing)"
		} else {
			why += " (Windows Update)"
		}
		add(Check{ID: "pending-reboot", Title: "No pending restart", Severity: Blocker,
			Detail:   why + ". Sysprep is likely to fail while servicing operations are waiting for a restart.",
			NextStep: "Restart Windows, let it finish installing updates, then run \"Ready for buyer\" again."})
	case f.PendingFileRename:
		add(Check{ID: "pending-reboot", Title: "No pending restart", Severity: Attention,
			Detail: "File rename operations are pending. This is common and often harmless, but it can interfere " +
				"with Sysprep on some systems.",
			NextStep: "Optional but recommended: restart Windows once before the handover."})
	default:
		add(Check{ID: "pending-reboot", Title: "No pending restart", Severity: Pass,
			Detail: "No pending restart indicators were found."})
	}

	// 7. Local accounts beyond the built-in ones. The reseal does not remove
	// accounts, so any extra account survives the handover and would be
	// handed to the buyer; WinPrep never deletes accounts itself, so the
	// operator must remove them first.
	switch {
	case f.AccountsErr != nil:
		add(Check{ID: "accounts", Title: "No extra local accounts", Severity: UnknownSeverity,
			Detail: "Could not list local accounts: " + f.AccountsErr.Error(),
			NextStep: "WinPrep will not reseal without verifying the account list. Restart and try again; if it " +
				"persists, report this as a bug."})
	case len(f.Accounts) == 0:
		add(Check{ID: "accounts", Title: "No extra local accounts", Severity: Pass,
			Detail: "Only built-in Windows accounts exist."})
	default:
		names := make([]string, 0, len(f.Accounts))
		for _, a := range f.Accounts {
			label := a.Name
			if a.Disabled {
				label += " (disabled)"
			}
			names = append(names, label)
		}
		add(Check{ID: "accounts", Title: "No extra local accounts", Severity: Blocker,
			Detail: "Additional local accounts exist: " + strings.Join(names, ", ") + ". They survive the reseal " +
				"and would be handed to the buyer with the laptop.",
			NextStep: "Remove these accounts in Settings > Accounts (or lusrmgr.msc) and let Windows delete their " +
				"data, then run \"Clean & Finish\" again. WinPrep never deletes accounts itself."})
	}

	// 8. User profiles beyond the built-in set. Profile data survives the
	// reseal, so leftover profiles or orphaned profile folders undermine the
	// handover and must be removed through Windows' own profile management.
	switch {
	case f.ProfilesErr != nil:
		add(Check{ID: "profiles", Title: "No extra user profiles", Severity: UnknownSeverity,
			Detail:   "Could not list user profiles: " + f.ProfilesErr.Error(),
			NextStep: "WinPrep will not reseal without verifying the profile list. Restart and try again."})
	case len(f.Profiles) == 0 && len(f.OrphanDirs) == 0:
		add(Check{ID: "profiles", Title: "No extra user profiles", Severity: Pass,
			Detail: "Only built-in profiles are present."})
	default:
		var details []string
		for _, p := range f.Profiles {
			details = append(details, fmt.Sprintf("%s (%s)", p.SID, p.Path))
		}
		for _, d := range f.OrphanDirs {
			details = append(details, fmt.Sprintf("folder without a registered profile: %s", d))
		}
		add(Check{ID: "profiles", Title: "No extra user profiles", Severity: Blocker,
			Detail: "Additional user data is present: " + strings.Join(details, "; ") + ". Profile folders survive " +
				"the reseal and WinPrep does not delete them.",
			NextStep: "Remove these profiles through Windows (Settings > Accounts, or System Properties > " +
				"User Profiles) so their data is cleaned up by Windows itself, then run \"Clean & Finish\" again."})
	}

	return checks
}

// sysprepActive reports whether the Sysprep logs were written recently enough
// that an operation looks to be in progress. The window is generous (90
// seconds) on purpose: a false "active" only delays the handover until the
// logs age out, while a false "idle" could overlap two operations.
func sysprepActive(f Facts) bool {
	for _, t := range f.SysprepLogTimes {
		age := f.Now.Sub(t)
		if age >= 0 && age <= 90*time.Second {
			return true
		}
	}
	return false
}

// Issues returns only the checks that need operator attention, in check order.
func (r PreflightResult) Issues() []Check {
	var out []Check
	for _, c := range r.Checks {
		if c.Severity != Pass {
			out = append(out, c)
		}
	}
	return out
}

// Explain renders the preflight result as plain text lines, used by the CLI
// and the fallback menu. The TUI renders the structured checks itself.
func (r PreflightResult) Explain() []string {
	lines := []string{fmt.Sprintf("Overall: %s", r.Verdict)}
	for _, c := range r.Checks {
		lines = append(lines, "", fmt.Sprintf("%s: %s", c.Title, c.Severity))
		if c.Detail != "" {
			lines = append(lines, "  "+c.Detail)
		}
		if c.NextStep != "" {
			lines = append(lines, "  Next step: "+c.NextStep)
		}
	}
	return lines
}
