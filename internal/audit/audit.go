// Package audit determines which Windows deployment mode the machine is in.
//
// WinPrep's supported workflow depends on Windows Audit Mode, so detection
// must be conservative: it is far worse to misidentify a deployed system as
// being in Audit Mode than to report an uncertain state. Uncertainty disables
// state-changing operations rather than guessing.
//
// Detection combines several documented signals instead of trusting any single
// registry value:
//
//   - HKLM\SOFTWARE\Microsoft\Windows\CurrentVersion\Setup\State\StateName:
//     IMAGE_STATE_COMPLETE means setup finished; IMAGE_STATE_UNDEPLOYABLE
//     means the image is mid-deployment (Audit Mode or OOBE).
//     (Microsoft, "Sysprep process overview")
//   - HKLM\SYSTEM\Setup\CmdLine: "audit.exe" while Audit Mode is active,
//     "oobe.exe" while OOBE runs.
//   - HKLM\SYSTEM\Setup\SystemSetupInProgress and OOBEInProgress: nonzero
//     while setup is in flight, used as corroboration only.
//   - The interactive user's SID: Audit Mode signs in the built-in
//     Administrator (RID 500), so an ordinary userRID alongside audit-mode
//     signals is treated as suspicious rather than as confirmation.
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
	// StateOOBE means Windows first-time setup (OOBE) is in progress.
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
	Weight string // how this signal was used: "primary", "corroborating", "conflicting"
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
}

// NewService returns an audit service backed by the given dependencies.
func NewService(reg platform.Registry, sys platform.System) *Service {
	return &Service{Reg: reg, Sys: sys}
}

// Collect reads the raw signals from the system. Missing values are recorded
// as such; a hard read failure returns an error.
func (s *Service) Collect(ctx context.Context) (platform.AuditSignals, error) {
	var sig platform.AuditSignals

	if v, ok, err := s.Reg.StringValue(platform.HKLM,
		`SOFTWARE\Microsoft\Windows\CurrentVersion\Setup\State`, "StateName"); err != nil {
		return sig, fmt.Errorf("reading setup state: %w", err)
	} else {
		sig.SetupStateName, sig.HaveSetupStateName = v, ok
	}

	if v, ok, err := s.Reg.StringValue(platform.HKLM,
		`SYSTEM\Setup`, "CmdLine"); err != nil {
		return sig, fmt.Errorf("reading setup command line: %w", err)
	} else {
		sig.SetupCmdLine, sig.HaveSetupCmdLine = v, ok
	}

	if v, ok, err := s.Reg.DWORDValue(platform.HKLM,
		`SYSTEM\Setup`, "SystemSetupInProgress"); err != nil {
		return sig, fmt.Errorf("reading SystemSetupInProgress: %w", err)
	} else {
		sig.SystemSetupInProgress, sig.HaveSystemSetupInProgress = v, ok
	}

	if v, ok, err := s.Reg.DWORDValue(platform.HKLM,
		`SYSTEM\Setup`, "OOBEInProgress"); err != nil {
		return sig, fmt.Errorf("reading OOBEInProgress: %w", err)
	} else {
		sig.OOBEInProgress, sig.HaveOOBEInProgress = v, ok
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

const (
	stateComplete     = "IMAGE_STATE_COMPLETE"
	stateUndeployable = "IMAGE_STATE_UNDEPLOYABLE"
	cmdAudit          = "audit.exe"
	cmdOOBE           = "oobe.exe"
)

// Evaluate applies the detection rules to a set of signals. It is pure so it
// can be exhaustively unit tested.
//
// Rules, in order:
//
//  1. StateName IMAGE_STATE_COMPLETE + audit.exe anywhere = conflict ->
//     StateUncertain (a completed image should not carry an audit CmdLine).
//  2. StateName IMAGE_STATE_COMPLETE otherwise -> StateNormalInstalled.
//  3. StateName IMAGE_STATE_UNDEPLOYABLE + CmdLine audit.exe + built-in
//     Administrator session -> StateAuditMode.
//  4. StateName IMAGE_STATE_UNDEPLOYABLE + CmdLine audit.exe + a different
//     user -> StateUncertain (unexpected for real Audit Mode).
//  5. StateName IMAGE_STATE_UNDEPLOYABLE + CmdLine oobe.exe or OOBEInProgress
//     = 1 -> StateOOBE.
//  6. StateName IMAGE_STATE_UNDEPLOYABLE with incomplete corroboration ->
//     StateUncertain.
//  7. Missing or unrecognized StateName -> StateUnknown.
func Evaluate(sig platform.AuditSignals) Assessment {
	a := Assessment{Signals: []Signal{}}

	state := strings.TrimSpace(sig.SetupStateName)
	cmdline := strings.ToLower(strings.TrimSpace(sig.SetupCmdLine))
	isAuditCmd := sig.HaveSetupCmdLine && cmdline == cmdAudit
	isOOBECmd := sig.HaveSetupCmdLine && cmdline == cmdOOBE

	record := func(name, value, weight string) {
		a.Signals = append(a.Signals, Signal{Name: name, Value: value, Weight: weight})
	}

	switch {
	case sig.HaveSetupStateName && state == stateComplete:
		record("Setup state (StateName)", state, "primary")
		if isAuditCmd {
			record("Setup command line", sig.SetupCmdLine, "conflicting")
			a.State = StateUncertain
			a.Summary = "Windows reports setup as complete, but the setup command line still names audit.exe. " +
				"These signals disagree, so WinPrep cannot safely treat this as Audit Mode."
			return a
		}
		a.State = StateNormalInstalled
		a.Summary = "Windows setup is complete. This is a normally deployed installation, not Audit Mode."

	case sig.HaveSetupStateName && state == stateUndeployable:
		record("Setup state (StateName)", state, "primary")
		if isAuditCmd {
			record("Setup command line", sig.SetupCmdLine, "primary")
			weight := "corroborating"
			if sig.CurrentUserSID != "" {
				if sig.CurrentUserIsBuiltinAdmin {
					record("Session user", "built-in Administrator ("+sig.CurrentUserSID+")", weight)
					a.State = StateAuditMode
					a.Summary = "Windows is running in Audit Mode: setup is mid-deployment, the setup command line names " +
						"audit.exe, and the session runs as the built-in Administrator."
					return a
				}
				record("Session user", sig.CurrentUserSID, "conflicting")
				a.State = StateUncertain
				a.Summary = "Setup state and the setup command line look like Audit Mode, but the session user is not " +
					"the built-in Administrator, which is unexpected for a genuine Audit Mode session."
				return a
			}
			record("Session user", "unknown (could not read user SID)", "corroborating")
			a.State = StateUncertain
			a.Summary = "Setup state and the setup command line look like Audit Mode, but the session user could not be " +
				"verified, so WinPrep will not assume Audit Mode."
			return a
		}
		if isOOBECmd || (sig.HaveOOBEInProgress && sig.OOBEInProgress == 1) {
			if isOOBECmd {
				record("Setup command line", sig.SetupCmdLine, "primary")
			}
			if sig.HaveOOBEInProgress {
				record("OOBE in progress", fmt.Sprintf("%d", sig.OOBEInProgress), "corroborating")
			}
			a.State = StateOOBE
			a.Summary = "Windows first-time setup (OOBE) is in progress. The machine has not been through initial setup yet."
			return a
		}
		if sig.HaveSystemSetupInProgress {
			record("SystemSetupInProgress", fmt.Sprintf("%d", sig.SystemSetupInProgress), "corroborating")
		}
		if sig.HaveOOBEInProgress {
			record("OOBEInProgress", fmt.Sprintf("%d", sig.OOBEInProgress), "corroborating")
		}
		a.State = StateUncertain
		a.Summary = "Windows reports setup as not finalized, but the signals available do not clearly identify " +
			"Audit Mode or OOBE. WinPrep will not guess."

	default:
		if sig.HaveSetupStateName {
			record("Setup state (StateName)", sig.SetupStateName, "unrecognized")
		} else {
			record("Setup state (StateName)", "not present", "conflicting")
		}
		a.State = StateUnknown
		a.Summary = "The Windows setup state could not be read or was not recognized. WinPrep cannot determine the " +
			"deployment mode."
	}
	return a
}
