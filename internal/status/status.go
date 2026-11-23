// Package status implements the read-only "System information" view: a
// summary of the Windows environment and whether the machine fits WinPrep's
// supported workflow.
package status

import (
	"context"
	"fmt"

	"github.com/developerasaad/winprep/internal/audit"
	"github.com/developerasaad/winprep/internal/format"
	"github.com/developerasaad/winprep/internal/platform"
)

// Report is the assembled status result.
type Report struct {
	Info     platform.SystemInfo
	User     platform.UserInfo
	Audit    audit.Assessment
	Suitable bool
	// SuitabilityNote explains the Suitable value in one sentence.
	SuitabilityNote string
	// Blocking lists conditions that prevent WinPrep's workflow on this
	// machine, if any.
	Blocking []string
}

// Service assembles the status report.
type Service struct {
	Sys   platform.System
	Audit *audit.Service
}

// NewService returns a status service.
func NewService(sys platform.System, a *audit.Service) *Service {
	return &Service{Sys: sys, Audit: a}
}

// Inspect collects and evaluates the status. All of it is read-only.
func (s *Service) Inspect(ctx context.Context) (Report, error) {
	var r Report

	info, err := s.Sys.SystemInfo(ctx)
	if err != nil {
		return r, err
	}
	r.Info = info

	user, err := s.Sys.UserInfo(ctx)
	if err != nil {
		return r, err
	}
	r.User = user

	assessment, err := s.Audit.Inspect(ctx)
	if err != nil {
		return r, err
	}
	r.Audit = assessment

	// Machine-level blockers first; they are independent of the setup state.
	switch {
	case !info.IsWindows:
		r.Blocking = append(r.Blocking, "This is not a Windows system; WinPrep supports Windows 10 and 11 only.")
	case info.IsServer():
		r.Blocking = append(r.Blocking, "Windows Server is not supported; WinPrep targets the client editions of Windows 10 and 11.")
	case info.Architecture != "x64":
		r.Blocking = append(r.Blocking, fmt.Sprintf("WinPrep currently ships x64 builds only; this system reports %s.", info.Architecture))
	}

	switch assessment.State {
	case audit.StateAuditMode:
		if len(r.Blocking) == 0 {
			r.Suitable = true
			r.SuitabilityNote = "This machine is in Audit Mode and can use WinPrep's full workflow."
		} else {
			r.SuitabilityNote = "The setup state is suitable, but see the blocking conditions below."
		}
	case audit.StateNormalInstalled:
		r.SuitabilityNote = "This Windows installation is already deployed, so WinPrep's Audit Mode workflow does not apply here."
		r.Blocking = append(r.Blocking, "Not in Audit Mode. Prepare machines by entering Audit Mode during Windows setup (see docs/workflow.md).")
	case audit.StateOOBE:
		r.SuitabilityNote = "Windows first-time setup is in progress. This is the right moment to enter Audit Mode."
		r.Blocking = append(r.Blocking, "Press Ctrl+Shift+F3 in Windows setup to enter Audit Mode before using WinPrep.")
	default:
		r.SuitabilityNote = "The setup state could not be determined with confidence."
		r.Blocking = append(r.Blocking, "WinPrep cannot confirm Audit Mode, so state-changing actions stay disabled.")
	}
	return r, nil
}

// Lines renders the report as plain text lines, shared by the CLI, the
// fallback menu, and the TUI (which styles them further).
func (r Report) Lines() []string {
	var lines []string
	add := func(format string, a ...any) { lines = append(lines, fmt.Sprintf(format, a...)) }

	add("Windows:      %s", valueOrUnknown(r.Info.OSName))
	if r.Info.DisplayVersion != "" {
		add("Version:      %s", r.Info.DisplayVersion)
	}
	add("Build:        %s", valueOrUnknown(r.Info.Build))
	computer := valueOrUnknown(joinNonEmpty(r.Info.Manufacturer, r.Info.Model, " "))
	add("Computer:     %s", computer)
	add("Processor:    %s", valueOrUnknown(r.Info.CPU))
	if r.Info.TotalRAMBytes > 0 {
		add("Memory:       %s", format.Bytes(r.Info.TotalRAMBytes))
	} else {
		add("Memory:       (not reported)")
	}
	add("Architecture: %s", valueOrUnknown(r.Info.Architecture))

	userLabel := r.User.Name
	switch {
	case userLabel == "":
		userLabel = "(not available)"
	case r.User.IsBuiltinAdmin:
		userLabel += " (built-in Administrator account)"
	}
	add("User:         %s", userLabel)
	if r.User.IsElevated {
		add("Privileges:   Running as administrator (elevated)")
	} else {
		add("Privileges:   Not elevated")
	}

	lines = append(lines, "")
	add("Setup state:  %s", r.Audit.State.String())
	for _, sig := range r.Audit.Signals {
		add("  Signal:     %s = %s (%s)", sig.Name, sig.Value, sig.Weight)
	}
	lines = append(lines, "")
	add("Summary:      %s", r.Audit.Summary)

	lines = append(lines, "")
	if r.Suitable {
		add("Suitable for the WinPrep workflow: Yes")
	} else {
		add("Suitable for the WinPrep workflow: No")
	}
	if len(r.Blocking) > 0 {
		add("Blocking conditions:")
		for _, b := range r.Blocking {
			add("  - %s", b)
		}
	}
	return lines
}

func valueOrUnknown(s string) string {
	if s == "" {
		return "(not available)"
	}
	return s
}

func joinNonEmpty(a, b, sep string) string {
	switch {
	case a == "":
		return b
	case b == "":
		return a
	default:
		return a + sep + b
	}
}
