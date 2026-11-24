// Package audit determines which Windows deployment mode the machine is in.
//
// WinPrep's supported workflow depends on Windows Audit Mode, so detection
// must be conservative: it is far worse to misidentify a deployed system as
// being in Audit Mode than to report an uncertain state. Uncertainty disables
// state-changing operations rather than guessing.
//
// Detection collects independent signals first and evaluates them afterwards.
// Each signal records whether it was available, absent, contradictory, or
// unreadable, and the assessment explains which observations produced the
// verdict, so a rejected machine can be diagnosed from its own report.
//
// The signals and the evidence behind them:
//
//   - HKLM\SOFTWARE\Microsoft\Windows\CurrentVersion\Setup\State\ImageState:
//     the setup-state value real Windows installations write. The prose of
//     Microsoft's "Windows Setup States" page calls the value StateName, but
//     the page's own examples and every field report (deployment tooling,
//     OEM images, support threads) use ImageState; StateName has not been
//     observed on a real machine, so both names are read. Documented states:
//     IMAGE_STATE_COMPLETE (deployed installation), IMAGE_STATE_UNDEPLOYABLE
//     (setup not finalized: Audit Mode, OOBE, or a failed phase), and
//     IMAGE_STATE_{GENERALIZE,SPECIALIZE}_RESEAL_TO_{OOBE,AUDIT} (setup will
//     continue into that mode at the next boot). (Microsoft, "Windows Setup
//     States", learn.microsoft.com/windows-hardware/manufacture/desktop/windows-setup-states)
//   - %WINDIR%\Setup\State\State.ini: the file mirror of the same state
//     (section [State], key ImageState). Read as a fallback and cross-check.
//   - HKLM\SYSTEM\Setup\Status\AuditBoot (and the AuditBootVolatile sibling
//     observed on Windows 11): 1 while the machine keeps booting into Audit
//     Mode. Set by the Sysprep /audit reseal; machines reported "trapped" in
//     Audit Mode carry 1 until an /oobe reseal clears it. Not officially
//     documented by Microsoft, but confirmed by multiple independent field
//     reports on Windows 7 through 10 and by the setup engine's design:
//     Microsoft documents that a machine configured for Audit Mode "will
//     continue to boot to audit mode by default" ("Boot Windows to Audit Mode
//     or OOBE"), which requires exactly such a persisted flag.
//   - HKLM\SYSTEM\Setup\OOBEInProgress: nonzero while first-time setup is on
//     screen. Together with IMAGE_STATE_UNDEPLOYABLE it identifies live OOBE.
//   - HKLM\SYSTEM\Setup\CmdLine and SystemSetupInProgress: collected as
//     observations. CmdLine is a generic "run before logon" slot (field
//     reports show oobe\windeploy.exe, SetupComplete.cmd paths, or empty); it
//     is not an audit-specific signal and never gates evaluation.
//   - The session user's SID: Microsoft documents that Audit Mode "starts the
//     computer in the built-in administrator account" ("Boot Windows to Audit
//     Mode or OOBE"), so the built-in Administrator (RID 500) session is the
//     corroborating evidence that distinguishes a live Audit Mode desktop
//     from a machine that is merely configured to enter Audit Mode at the
//     next boot.
//
// Audit Mode persists across restarts and shutdowns by design; no tooling is
// needed to keep it alive, only to leave it (via Sysprep /oobe).
package audit

import (
	"context"
	"fmt"
	"strings"

	"github.com/developerasaad/winprep/internal/platform"
)

// State is the evaluated deployment state.
type State int

const (
	// StateUnknown means the signals did not allow any conclusion.
	StateUnknown State = iota
	// StateAuditMode means Audit Mode is confirmed by multiple consistent
	// signals.
	StateAuditMode
	// StateOOBE means Windows first-time setup (OOBE) is in progress or
	// configured as the next setup phase.
	StateOOBE
	// StateNormalInstalled means setup completed; this is a deployed Windows
	// installation.
	StateNormalInstalled
	// StateUncertain means some setup-related signals were present but they
	// conflicted or were incomplete. State-changing operations must not run.
	StateUncertain
)

func (s State) String() string {
	switch s {
	case StateAuditMode:
		return "Audit Mode (confirmed)"
	case StateOOBE:
		return "First-time setup (OOBE) in progress"
	case StateNormalInstalled:
		return "Normal Windows installation (setup complete)"
	case StateUncertain:
		return "Setup state uncertain"
	default:
		return "Setup state unknown"
	}
}

// Short returns a compact label for the main menu header.
func (s State) Short() string {
	switch s {
	case StateAuditMode:
		return "Audit Mode confirmed"
	case StateOOBE:
		return "First-time setup (OOBE)"
	case StateNormalInstalled:
		return "Normal installation"
	case StateUncertain:
		return "Setup state uncertain"
	default:
		return "Setup state unknown"
	}
}

// Signal is one observation used in the evaluation, kept in the assessment so
// the UI can show exactly what was seen.
type Signal struct {
	Name   string
	Value  string
	Weight string // how this signal was used: "primary", "corroborating", "conflicting", "observation"
}

// Assessment is the outcome of evaluating the collected signals.
type Assessment struct {
	State   State
	Summary string
	Signals []Signal
}

// Service collects and evaluates audit signals.
type Service struct {
	Reg platform.Registry
	Sys platform.System
	// Ini reads %WINDIR%\Setup\State\State.ini. It may be nil, in which case
	// the file is simply not consulted (unit tests that do not exercise the
	// fallback use this).
	Ini platform.SetupStateFile
}

// NewService returns an audit service backed by the given dependencies.
func NewService(reg platform.Registry, sys platform.System, ini platform.SetupStateFile) *Service {
	return &Service{Reg: reg, Sys: sys, Ini: ini}
}

const (
	setupStateKey = `SOFTWARE\Microsoft\Windows\CurrentVersion\Setup\State`
	systemSetup   = `SYSTEM\Setup`
	setupStatus   = `SYSTEM\Setup\Status`
)

// Collect reads the raw signals from the system. Missing values are recorded
// as such; a hard read failure returns an error. Read failures are not
// silently treated as absence, because "unreadable" and "absent" lead to
// different operator guidance.
func (s *Service) Collect(ctx context.Context) (platform.AuditSignals, error) {
	var sig platform.AuditSignals

	if v, ok, err := s.Reg.StringValue(platform.HKLM, setupStateKey, "ImageState"); err != nil {
		return sig, fmt.Errorf("reading setup state (ImageState): %w", err)
	} else {
		sig.SetupImageState, sig.HaveSetupImageState = v, ok
	}

	// The documented-but-unobserved alternate name. A value here that agrees
	// with ImageState is harmless; one that disagrees is a conflict the
	// evaluation reports.
	if v, ok, err := s.Reg.StringValue(platform.HKLM, setupStateKey, "StateName"); err != nil {
		return sig, fmt.Errorf("reading setup state (StateName): %w", err)
	} else {
		sig.SetupStateName, sig.HaveSetupStateName = v, ok
	}

	if s.Ini != nil {
		content, ok, err := s.Ini.StateIniContent(ctx)
		switch {
		case err != nil:
			sig.StateIniErr = err
		case ok:
			if v, present := parseStateIni(content); present {
				sig.StateIniImageState, sig.HaveStateIniImageState = v, true
			}
		}
	}

	if v, ok, err := s.Reg.StringValue(platform.HKLM, systemSetup, "CmdLine"); err != nil {
		return sig, fmt.Errorf("reading setup command line: %w", err)
	} else {
		sig.SetupCmdLine, sig.HaveSetupCmdLine = v, ok
	}

	if v, ok, err := s.Reg.DWORDValue(platform.HKLM, systemSetup, "SystemSetupInProgress"); err != nil {
		return sig, fmt.Errorf("reading SystemSetupInProgress: %w", err)
	} else {
		sig.SystemSetupInProgress, sig.HaveSystemSetupInProgress = v, ok
	}

	if v, ok, err := s.Reg.DWORDValue(platform.HKLM, systemSetup, "OOBEInProgress"); err != nil {
		return sig, fmt.Errorf("reading OOBEInProgress: %w", err)
	} else {
		sig.OOBEInProgress, sig.HaveOOBEInProgress = v, ok
	}

	if v, ok, err := s.Reg.DWORDValue(platform.HKLM, setupStatus, "AuditBoot"); err != nil {
		return sig, fmt.Errorf("reading AuditBoot: %w", err)
	} else {
		sig.AuditBoot, sig.HaveAuditBoot = v, ok
	}

	if v, ok, err := s.Reg.DWORDValue(platform.HKLM, setupStatus, "AuditBootVolatile"); err != nil {
		return sig, fmt.Errorf("reading AuditBootVolatile: %w", err)
	} else {
		sig.AuditBootVolatile, sig.HaveAuditBootVolatile = v, ok
	}

	if u, err := s.Sys.UserInfo(ctx); err != nil {
		return sig, fmt.Errorf("reading user info: %w", err)
	} else {
		sig.CurrentUserSID = u.SID
		sig.CurrentUserIsBuiltinAdmin = u.IsBuiltinAdmin
	}
	return sig, nil
}

// Inspect collects signals and evaluates them in one step.
func (s *Service) Inspect(ctx context.Context) (Assessment, error) {
	sig, err := s.Collect(ctx)
	if err != nil {
		return Assessment{}, err
	}
	return Evaluate(sig), nil
}

// parseStateIni extracts the ImageState entry from a State.ini file of the
// documented shape:
//
//	[State]
//	ImageState="IMAGE_STATE_COMPLETE"
//
// Section and key names are matched case-insensitively, and the value is
// trimmed of spaces and quotes. A missing [State] section or ImageState key
// yields ok=false; the StateName key is accepted as the same documented-name
// fallback the registry read uses.
func parseStateIni(content string) (string, bool) {
	inState := false
	for _, raw := range strings.Split(content, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			inState = strings.EqualFold(strings.TrimSpace(line[1:len(line)-1]), "State")
			continue
		}
		if !inState {
			continue
		}
		key, value, found := strings.Cut(line, "=")
		if !found {
			continue
		}
		key = strings.TrimSpace(key)
		if !strings.EqualFold(key, "ImageState") && !strings.EqualFold(key, "StateName") {
			continue
		}
		value = strings.TrimSpace(value)
		value = strings.Trim(value, `"`)
		value = strings.TrimSpace(value)
		if value != "" {
			return value, true
		}
	}
	return "", false
}

// The documented IMAGE_STATE_* values, plus the classifications used by the
// rules. IMAGE_STATE_UNDEPLOYABLE is shared by Audit Mode and OOBE sessions
// (it means "setup not finalized"), so it is an audit-family state that
// still needs the other signals to discriminate.
const (
	stateComplete = "IMAGE_STATE_COMPLETE"

	stateUndeployable          = "IMAGE_STATE_UNDEPLOYABLE"
	stateSpecializeResealAudit = "IMAGE_STATE_SPECIALIZE_RESEAL_TO_AUDIT"
	stateGeneralizeResealAudit = "IMAGE_STATE_GENERALIZE_RESEAL_TO_AUDIT"
	stateSpecializeResealOOBE  = "IMAGE_STATE_SPECIALIZE_RESEAL_TO_OOBE"
	stateGeneralizeResealOOBE  = "IMAGE_STATE_GENERALIZE_RESEAL_TO_OOBE"
)

// Evaluate applies the detection rules to a set of signals. It is pure so it
// can be exhaustively unit tested.
//
// Rules, in order:
//
//  1. A disagreement between the registry state, the StateName fallback, and
//     State.ini is a conflict: StateUncertain.
//  2. IMAGE_STATE_COMPLETE with the audit boot flag or live OOBE is a
//     conflict: StateUncertain. Otherwise: StateNormalInstalled.
//  3. A RESEAL_TO_OOBE state is OOBE (with the audit boot flag also set that
//     is a conflict: StateUncertain).
//  4. IMAGE_STATE_UNDEPLOYABLE with OOBEInProgress set is live OOBE.
//  5. Otherwise an audit-family state (UNDEPLOYABLE or RESEAL_TO_AUDIT) is
//     confirmed Audit Mode only when three things agree: the audit boot
//     flag is set, OOBE is not in progress, and the session runs as the
//     built-in Administrator. Missing or contradicting corroboration yields
//     StateUncertain with the failing check named in the summary.
//  6. No state value at all, or an unrecognized one: StateUnknown.
//
// CmdLine and SystemSetupInProgress are recorded as observations and never
// gate the outcome; see the package comment for why.
func Evaluate(sig platform.AuditSignals) Assessment {
	a := Assessment{Signals: []Signal{}}

	record := func(name, value, weight string) {
		a.Signals = append(a.Signals, Signal{Name: name, Value: value, Weight: weight})
	}
	present := func(have bool, value string) string {
		if !have {
			return "not present"
		}
		if value == "" {
			return "(empty)"
		}
		return value
	}

	stateStr, stateSource := resolveState(sig)
	if stateSource != "" {
		record("Setup state ("+stateSource+")", stateStr, "primary")
	}

	// Observation-only signals. They are reported so the operator (and bug
	// reports) see the full picture, even where the rules do not use them.
	record("Setup command line", present(sig.HaveSetupCmdLine, sig.SetupCmdLine), "observation")
	if sig.HaveSystemSetupInProgress {
		record("SystemSetupInProgress", fmt.Sprintf("%d", sig.SystemSetupInProgress), "observation")
	}
	if sig.HaveOOBEInProgress {
		record("OOBEInProgress", fmt.Sprintf("%d", sig.OOBEInProgress), "corroborating")
	}
	auditBoot := (sig.HaveAuditBoot && sig.AuditBoot != 0) ||
		(sig.HaveAuditBootVolatile && sig.AuditBootVolatile != 0)
	if sig.HaveAuditBoot || sig.HaveAuditBootVolatile {
		record("Audit boot flag", onOff(auditBoot), "corroborating")
	}

	oobeRunning := sig.HaveOOBEInProgress && sig.OOBEInProgress != 0
	admin := sig.CurrentUserSID != "" && sig.CurrentUserIsBuiltinAdmin

	switch {
	// 1. The documented sources disagree with each other.
	case sourceConflict(sig):
		a.State = StateUncertain
		a.Summary = "The Windows setup-state sources disagree (registry ImageState, StateName, and State.ini " +
			"do not match), so WinPrep cannot safely determine the deployment mode."

	// 6. Nothing readable at all.
	case stateSource == "":
		detail := "The Windows setup state could not be read from the registry or State.ini"
		if sig.StateIniErr != nil {
			detail += " (reading State.ini failed: " + sig.StateIniErr.Error() + ")"
		}
		a.State = StateUnknown
		a.Summary = detail + ". WinPrep cannot determine the deployment mode."

	// 6. A state value exists but matches nothing documented.
	case stateStr != stateComplete && !auditFamilyState(stateStr) && !oobeFamilyState(stateStr):
		record("Setup state ("+stateSource+")", stateStr, "unrecognized")
		a.State = StateUnknown
		a.Summary = fmt.Sprintf("The Windows setup state %q is not one of the documented IMAGE_STATE values, so "+
			"WinPrep cannot determine the deployment mode.", stateStr)

	// 2. Setup completed.
	case stateStr == stateComplete:
		switch {
		case auditBoot:
			record("Audit boot flag", onOff(true), "conflicting")
			a.State = StateUncertain
			a.Summary = "Windows reports setup as complete, but the machine is still flagged to boot into Audit " +
				"Mode. These signals disagree, so WinPrep cannot safely treat this as a deployed installation."
		case oobeRunning:
			record("OOBEInProgress", fmt.Sprintf("%d", sig.OOBEInProgress), "conflicting")
			a.State = StateUncertain
			a.Summary = "Windows reports setup as complete while first-time setup is marked in progress. These " +
				"signals disagree, so WinPrep cannot determine the deployment mode."
		default:
			a.State = StateNormalInstalled
			a.Summary = "Windows setup is complete (ImageState is IMAGE_STATE_COMPLETE). This is a normally " +
				"deployed installation, not Audit Mode."
		}

	// 3. Headed into first-time setup.
	case oobeFamilyState(stateStr):
		if auditBoot {
			record("Audit boot flag", onOff(true), "conflicting")
			a.State = StateUncertain
			a.Summary = "Windows is configured to continue into first-time setup (OOBE), but the machine is also " +
				"flagged to boot into Audit Mode. These directions disagree, so WinPrep will not guess."
		} else {
			a.State = StateOOBE
			a.Summary = "Windows is configured to continue into first-time setup (OOBE) at the next setup phase. " +
				"The machine has not been through initial setup yet."
		}

	// 4. Live OOBE: UNDEPLOYABLE with first-time setup on screen right now.
	case stateStr == stateUndeployable && oobeRunning:
		a.State = StateOOBE
		a.Summary = "Windows first-time setup (OOBE) is running right now. The machine has not completed initial " +
			"setup yet."

	// 5. Audit-family state; require the corroborating evidence. A
	// RESEAL_TO_AUDIT value combined with live OOBE points in two directions
	// at once, which is a conflict, not a guess.
	case oobeRunning:
		record("OOBEInProgress", fmt.Sprintf("%d", sig.OOBEInProgress), "conflicting")
		a.State = StateUncertain
		a.Summary = "The setup state points toward Audit Mode (" + stateStr + ") while first-time setup is marked " +
			"in progress. These signals disagree, so WinPrep cannot determine the deployment mode."

	case admin && auditBoot:
		record("Session user", "built-in Administrator ("+sig.CurrentUserSID+")", "corroborating")
		a.State = StateAuditMode
		a.Summary = "Windows is running in Audit Mode: the setup state is " + stateStr + ", the machine is flagged " +
			"to boot into Audit Mode, and the session runs as the built-in Administrator."

	case auditBoot && sig.CurrentUserSID == "":
		record("Session user", "unknown (could not read user SID)", "conflicting")
		a.State = StateUncertain
		a.Summary = "The setup state and the audit boot flag look like Audit Mode, but the session user could not " +
			"be verified, so WinPrep will not assume Audit Mode."

	case auditBoot:
		record("Session user", sig.CurrentUserSID, "conflicting")
		a.State = StateUncertain
		a.Summary = "The setup state and the audit boot flag look like Audit Mode, but the session user is not the " +
			"built-in Administrator. A genuine Audit Mode session signs in as the built-in Administrator; sign " +
			"in as that account and run WinPrep again."

	default:
		a.State = StateUncertain
		a.Summary = "The setup state (" + stateStr + ") is compatible with Audit Mode, but the audit boot flag " +
			"was not set (HKLM\\SYSTEM\\Setup\\Status\\AuditBoot), so WinPrep cannot confirm the machine is in " +
			"an Audit Mode session rather than another mid-setup state."
	}
	return a
}

// resolveState picks the setup-state value and remembers where it came from.
// Preference order: the registry ImageState value (what real Windows writes),
// the registry StateName value (the documented name), then State.ini.
func resolveState(sig platform.AuditSignals) (state, source string) {
	switch {
	case sig.HaveSetupImageState:
		return strings.TrimSpace(sig.SetupImageState), "registry ImageState"
	case sig.HaveSetupStateName:
		return strings.TrimSpace(sig.SetupStateName), "registry StateName"
	case sig.HaveStateIniImageState:
		return strings.TrimSpace(sig.StateIniImageState), "State.ini"
	default:
		return "", ""
	}
}

// sourceConflict reports whether two available state sources carry different
// values. Absent sources never conflict.
func sourceConflict(sig platform.AuditSignals) bool {
	registry := ""
	if sig.HaveSetupImageState {
		registry = strings.TrimSpace(sig.SetupImageState)
	} else if sig.HaveSetupStateName {
		registry = strings.TrimSpace(sig.SetupStateName)
	} else {
		return false // nothing to conflict with
	}
	if sig.HaveSetupImageState && sig.HaveSetupStateName &&
		strings.TrimSpace(sig.SetupImageState) != strings.TrimSpace(sig.SetupStateName) {
		return true
	}
	if sig.HaveStateIniImageState && strings.TrimSpace(sig.StateIniImageState) != registry {
		return true
	}
	return false
}

func auditFamilyState(state string) bool {
	return state == stateUndeployable ||
		state == stateSpecializeResealAudit ||
		state == stateGeneralizeResealAudit
}

func oobeFamilyState(state string) bool {
	return state == stateSpecializeResealOOBE ||
		state == stateGeneralizeResealOOBE
}

func onOff(b bool) string {
	if b {
		return "set (boot into Audit Mode)"
	}
	return "not set"
}
