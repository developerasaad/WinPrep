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

	"github.com/developerasaad/winprep/internal/artifacts"
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

	Accounts          []AccountFact
	AccountsErr       error
	Profiles          []ProfileFact
	ProfilesErr       error
	OrphanDirs        []string
	SysprepPresent    bool
	SysprepErr        error
	SysprepRunning    bool
	ProcessNames      []string
	ProcsErr          error
	PendingCBSReboot  bool
	PendingWUReboot   bool
	PendingFileRename bool
	RebootErr         error
	Artifacts         []artifacts.Entry
	ArtifactsErr      error
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

	// 5. Sysprep not already running.
	switch {
	case f.ProcsErr != nil:
		add(Check{ID: "sysprep-idle", Title: "Sysprep not already running", Severity: UnknownSeverity,
			Detail:   "Could not list running processes: " + f.ProcsErr.Error(),
			NextStep: "Close any Sysprep windows and try again."})
	case f.SysprepRunning:
		add(Check{ID: "sysprep-idle", Title: "Sysprep not already running", Severity: Blocker,
			Detail:   "A Sysprep process is already running. Two Sysprep operations must never overlap.",
			NextStep: "Finish or close the existing Sysprep process, then run \"Ready for buyer\" again."})
	default:
		add(Check{ID: "sysprep-idle", Title: "Sysprep not already running", Severity: Pass,
			Detail: "No Sysprep process is running."})
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

	// 7. Local accounts beyond the built-in ones. WinPrep never deletes
	// accounts; the operator must remove them manually.
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
		add(Check{ID: "accounts", Title: "No extra local accounts", Severity: Attention,
			Detail: "Additional local accounts exist: " + strings.Join(names, ", ") + ". Sysprep does not remove " +
				"accounts or their files, and WinPrep will not delete them for you.",
			NextStep: "Remove these accounts in Settings > Accounts (or netplwiz) and delete their files, then run " +
				"\"Ready for buyer\" again. To continue anyway, acknowledge this at the confirmation step."})
	}

	// 8. User profiles beyond the built-in set.
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
		add(Check{ID: "profiles", Title: "No extra user profiles", Severity: Attention,
			Detail: "Additional user data is present: " + strings.Join(details, "; ") + ". Sysprep does not remove " +
				"profile folders or their contents.",
			NextStep: "Review and remove these profiles and their folders manually before selling the laptop. To " +
				"continue anyway, acknowledge this at the confirmation step."})
	}

	// 9. WinPrep's own artifacts. Informational: they are removed during the
	// handover.
	switch {
	case f.ArtifactsErr != nil:
		add(Check{ID: "artifacts", Title: "WinPrep artifacts", Severity: UnknownSeverity,
			Detail:   "Could not read the WinPrep artifact manifest: " + f.ArtifactsErr.Error(),
			NextStep: "Check %ProgramData%\\WinPrep\\managed-artifacts.json manually, then try again."})
	case len(f.Artifacts) == 0:
		add(Check{ID: "artifacts", Title: "WinPrep artifacts", Severity: Pass,
			Detail: "WinPrep has no recorded changes to clean up."})
	default:
		paths := make([]string, 0, len(f.Artifacts))
		for _, e := range f.Artifacts {
			paths = append(paths, e.Path)
		}
		add(Check{ID: "artifacts", Title: "WinPrep artifacts", Severity: Pass,
			Detail: "WinPrep will remove its recorded changes: " + strings.Join(paths, ", ") + "."})
	}

	return checks
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
