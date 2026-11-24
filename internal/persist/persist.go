// Package persist implements "Prepare for display": verifying that a laptop
// in Audit Mode is ready to be shown to customers repeatedly.
//
// Audit Mode already survives restarts and shutdowns by Windows design, so
// the honest implementation is verification and explanation, not system
// modification. WinPrep deliberately does not create startup tasks, kill the
// Sysprep window, or apply "persistence hacks": they are unnecessary at best
// and can break a later reseal at worst. If a future version ever needs to
// apply a change, it must be recorded in the artifact manifest so "Ready for
// buyer" can remove it again.
package persist

import (
	"context"

	"github.com/developerasaad/winprep/internal/artifacts"
	"github.com/developerasaad/winprep/internal/audit"
	"github.com/developerasaad/winprep/internal/platform"
)

// Report is the outcome of a persist run.
type Report struct {
	Audit             audit.Assessment
	SysprepWindowOpen bool
	Artifacts         []artifacts.Entry
	// ReadyForDemo is true when the machine is in a supported Audit Mode
	// session with no WinPrep-managed leftovers.
	ReadyForDemo bool
	// Explanation states what was checked and what it means.
	Explanation []string
	// Guidance lists concrete next steps for the operator.
	Guidance []string
	// ChangesApplied lists modifications WinPrep made during this run. The
	// current implementation makes none; the field exists so the report is
	// explicit about it either way.
	ChangesApplied []artifacts.Entry
}

// Service runs the persist workflow.
type Service struct {
	Sys   platform.System
	Audit *audit.Service
	Procs platform.ProcessLister
	Store *artifacts.Store
}

// NewService returns a persist service.
func NewService(sys platform.System, a *audit.Service, procs platform.ProcessLister, store *artifacts.Store) *Service {
	return &Service{Sys: sys, Audit: a, Procs: procs, Store: store}
}

// Inspect evaluates the machine and produces the report. It never modifies
// the system, which also makes repeated runs trivially idempotent.
func (s *Service) Inspect(ctx context.Context) (Report, error) {
	var r Report

	assessment, err := s.Audit.Inspect(ctx)
	if err != nil {
		return r, err
	}
	r.Audit = assessment

	if m, err := s.Store.Load(); err == nil {
		r.Artifacts = m.Entries
	} else {
		// A manifest that cannot be read is worth surfacing: it may record
		// changes another WinPrep version applied.
		r.Explanation = append(r.Explanation,
			"Warning: the WinPrep artifact manifest could not be read ("+err.Error()+"). "+
				"If you have used WinPrep on this machine before, check %ProgramData%\\WinPrep manually.")
	}

	procs, procsErr := s.Procs.ProcessNames(ctx)
	if procsErr == nil {
		for _, p := range procs {
			if equalFoldASCII(p, "sysprep.exe") {
				r.SysprepWindowOpen = true
				break
			}
		}
	}

	switch assessment.State {
	case audit.StateAuditMode:
		r.ReadyForDemo = true
		r.Explanation = append(r.Explanation,
			"Audit Mode is active. Windows stays in Audit Mode across restarts and shutdowns on its own; "+
				"no extra configuration is needed to keep the laptop demonstrable.",
			"Customers can use the laptop and restart it freely. Each sign-in lands back in the same "+
				"Audit Mode session without completing Windows setup.",
			"Audit Mode signs in as the built-in Administrator. Anything a customer does on the machine "+
				"happens with full administrator rights; see docs/safety.md.")
		if r.SysprepWindowOpen {
			r.Explanation = append(r.Explanation,
				"The System Preparation Tool window (Sysprep) is currently open. You can close it yourself with "+
					"its close button; merely closing the window does not end Audit Mode.")
			r.Guidance = append(r.Guidance,
				"WinPrep does not close the Sysprep window automatically. Reliably telling an idle window from a "+
					"reseal in progress is not possible without risky assumptions, so closing it stays a manual step.")
		}
		if len(r.Artifacts) > 0 {
			r.Explanation = append(r.Explanation,
				"WinPrep has previously recorded changes on this machine. They will be removed automatically "+
					"during \"Clean & Finish\".")
		}
		r.Guidance = append(r.Guidance,
			"When you are ready to sell, run \"Clean & Finish\". It validates the machine, removes the "+
				"approved demonstration data and WinPrep's own artifacts, and reseals Windows into "+
				"first-time setup.",
			"Remove any accounts you created yourself first; WinPrep never deletes accounts. Files outside "+
				"the approved cleanup scope (Desktop, Documents, and similar) stay where they are.")
		r.ChangesApplied = nil
		r.Explanation = append(r.Explanation,
			"No system changes were made by this command.")

	case audit.StateOOBE:
		r.Explanation = append(r.Explanation,
			"Windows first-time setup (OOBE) is currently running. This is exactly the moment Audit Mode is "+
				"meant to be entered.")
		r.Guidance = append(r.Guidance,
			"Press Ctrl+Shift+F3 on the setup screen. The machine restarts into Audit Mode, signed in as the "+
				"built-in Administrator.",
			"Once Audit Mode is active, run \"Prepare for display\" again to verify the machine.")

	case audit.StateNormalInstalled:
		r.Explanation = append(r.Explanation,
			"This Windows installation is already deployed (setup is complete). It is not in Audit Mode, so "+
				"WinPrep's display-preparation workflow does not apply.")
		r.Guidance = append(r.Guidance,
			"Converting a machine in normal use into Audit Mode is not a supported or reliable operation. To use "+
				"the WinPrep workflow, reinstall Windows and enter Audit Mode before completing first-time setup "+
				"(see docs/workflow.md).")

	default:
		r.Explanation = append(r.Explanation,
			assessment.Summary)
		r.Guidance = append(r.Guidance,
			"WinPrep cannot confirm the setup state, so it will not change anything. Restart Windows and run "+
				"this command again; if the state stays unclear, report this as a bug.")
	}
	return r, nil
}

func equalFoldASCII(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		ca, cb := a[i], b[i]
		if 'A' <= ca && ca <= 'Z' {
			ca += 'a' - 'A'
		}
		if 'A' <= cb && cb <= 'Z' {
			cb += 'a' - 'A'
		}
		if ca != cb {
			return false
		}
	}
	return true
}

// Lines renders the report as plain text, shared by the CLI, the fallback
// menu, and the TUI.
func (r Report) Lines() []string {
	lines := []string{"Setup state: " + r.Audit.State.String()}
	for _, e := range r.Explanation {
		lines = append(lines, e)
	}
	if len(r.Guidance) > 0 {
		lines = append(lines, "", "What to do next:")
		for _, g := range r.Guidance {
			lines = append(lines, "  - "+g)
		}
	}
	if r.ReadyForDemo {
		lines = append(lines, "", "Result: This laptop is ready to be demonstrated to customers.")
	} else {
		lines = append(lines, "", "Result: Not ready for customer demonstrations yet; see the notes above.")
	}
	if len(r.ChangesApplied) == 0 {
		lines = append(lines, "No system changes were made.")
	}
	return lines
}
