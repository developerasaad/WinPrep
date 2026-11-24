package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/developerasaad/winprep/internal/app"
	"github.com/developerasaad/winprep/internal/audit"
	"github.com/developerasaad/winprep/internal/platform"
)

// runSignals prints every setup-state signal the detector collected, the raw
// value at its documented location, and the evaluation that followed. It is
// completely read-only: its purpose is troubleshooting a machine that a
// legitimate Audit Mode session was rejected on, and producing output that
// can be pasted into a bug report.
//
// It is deliberately not part of the two-action menu; it is a developer and
// support tool. The output contains SIDs and setup-state values only - no
// passwords, tokens, or personal files are read.
func runSignals(ctx context.Context, a *app.App, io IO) int {
	sig, err := a.Audit.Collect(ctx)
	if err != nil {
		return reportError(io, "collecting the setup-state signals", err)
	}
	assessment := audit.Evaluate(sig)

	fmt.Fprintln(io.Out, "WinPrep setup-state report (read-only)")
	fmt.Fprintln(io.Out, "====================================")
	fmt.Fprintln(io.Out)

	fmt.Fprintln(io.Out, "Raw signals, where the detector reads them:")
	for _, line := range rawSignalLines(sig) {
		fmt.Fprintln(io.Out, "  "+line)
	}
	fmt.Fprintln(io.Out)

	fmt.Fprintln(io.Out, "Evaluation:")
	fmt.Fprintln(io.Out, "  Setup state: "+assessment.State.String())
	for _, line := range wrapPlain(assessment.Summary, 74) {
		fmt.Fprintln(io.Out, "  "+line)
	}
	fmt.Fprintln(io.Out)

	fmt.Fprintln(io.Out, "How each signal was used:")
	for _, s := range assessment.Signals {
		fmt.Fprintf(io.Out, "  - %s: %s (%s)\n", s.Name, s.Value, s.Weight)
	}
	fmt.Fprintln(io.Out)

	fmt.Fprintln(io.Out, "No changes were made. If this machine should be in Audit Mode but the state")
	fmt.Fprintln(io.Out, "above says otherwise, paste this whole report into an issue at")
	fmt.Fprintln(io.Out, "https://github.com/developerasaad/WinPrep/issues together with the output of")
	fmt.Fprintln(io.Out, `"whoami /user" (run in the same window). See docs/troubleshooting.md first.`)
	return ExitOK
}

// rawSignalLines renders each collected value next to the exact place it was
// read from, including absent and unreadable ones: an honest report shows
// what was missing, not only what was found.
func rawSignalLines(sig platform.AuditSignals) []string {
	show := func(have bool, value string) string {
		switch {
		case !have:
			return "(not present)"
		case value == "":
			return "(empty)"
		default:
			return value
		}
	}
	lines := []string{
		`HKLM\SOFTWARE\Microsoft\Windows\CurrentVersion\Setup\State\ImageState = ` +
			show(sig.HaveSetupImageState, sig.SetupImageState),
		`HKLM\SOFTWARE\Microsoft\Windows\CurrentVersion\Setup\State\StateName = ` +
			show(sig.HaveSetupStateName, sig.SetupStateName),
	}
	switch {
	case sig.StateIniErr != nil:
		lines = append(lines, "%WINDIR%\\Setup\\State\\State.ini [State] ImageState = (unreadable: "+
			sig.StateIniErr.Error()+")")
	case sig.HaveStateIniImageState:
		lines = append(lines, "%WINDIR%\\Setup\\State\\State.ini [State] ImageState = "+sig.StateIniImageState)
	default:
		lines = append(lines, "%WINDIR%\\Setup\\State\\State.ini [State] ImageState = (not present)")
	}
	lines = append(lines,
		`HKLM\SYSTEM\Setup\CmdLine = `+show(sig.HaveSetupCmdLine, sig.SetupCmdLine),
		fmt.Sprintf(`HKLM\SYSTEM\Setup\SystemSetupInProgress = %s`, dwordText(sig.HaveSystemSetupInProgress, sig.SystemSetupInProgress)),
		fmt.Sprintf(`HKLM\SYSTEM\Setup\OOBEInProgress = %s`, dwordText(sig.HaveOOBEInProgress, sig.OOBEInProgress)),
		fmt.Sprintf(`HKLM\SYSTEM\Setup\Status\AuditBoot = %s`, dwordText(sig.HaveAuditBoot, sig.AuditBoot)),
		fmt.Sprintf(`HKLM\SYSTEM\Setup\Status\AuditBootVolatile = %s`, dwordText(sig.HaveAuditBootVolatile, sig.AuditBootVolatile)),
	)
	if sig.CurrentUserSID == "" {
		lines = append(lines, "Session user SID = (could not be read)")
	} else if sig.CurrentUserIsBuiltinAdmin {
		lines = append(lines, "Session user SID = "+sig.CurrentUserSID+" (built-in Administrator, RID 500)")
	} else {
		lines = append(lines, "Session user SID = "+sig.CurrentUserSID+" (not the built-in Administrator)")
	}
	return lines
}

func dwordText(have bool, value uint32) string {
	if !have {
		return "(not present)"
	}
	return fmt.Sprintf("%d", value)
}

// wrapPlain wraps text at width without any styling; a small local helper so
// the signals report does not depend on the TUI's wrapping.
func wrapPlain(s string, width int) []string {
	words := strings.Fields(s)
	if len(words) == 0 {
		return []string{""}
	}
	var lines []string
	line := words[0]
	for _, w := range words[1:] {
		if len(line)+1+len(w) > width {
			lines = append(lines, line)
			line = w
			continue
		}
		line += " " + w
	}
	return append(lines, line)
}
