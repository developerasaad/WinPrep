// Package cli implements the conventional command-line interface. It shares
// the same App services as the TUI; there is no separate business logic
// here, only argument parsing, prompts, and output formatting.
package cli

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/developerasaad/winprep/internal/app"
	"github.com/developerasaad/winprep/internal/handover"
	"github.com/developerasaad/winprep/internal/platform"
)

// Exit codes. They are part of the tool's contract and documented in help.
const (
	ExitOK    = 0
	ExitFail  = 1
	ExitUsage = 2
)

// IO carries the streams the CLI talks to. In production these are os
// .Std{out,err} and os.Stdin; tests inject buffers.
type IO struct {
	Out         io.Writer
	Err         io.Writer
	In          io.Reader
	Interactive bool // whether In can be prompted
}

// Run parses args and executes the command, returning the process exit code.
func Run(ctx context.Context, a *app.App, io IO, args []string) int {
	if len(args) == 0 {
		// The interactive entry point is handled by main; reaching the CLI
		// with no arguments means show help.
		fmt.Fprint(io.Out, helpText(a))
		return ExitOK
	}

	cmd := args[0]
	rest := args[1:]

	switch cmd {
	case "persist":
		return runPersist(ctx, a, io)
	case "clean":
		return runClean(ctx, a, io, rest)
	case "help", "--help", "-h", "/?":
		fmt.Fprint(io.Out, helpText(a))
		return ExitOK
	case "version", "--version", "-v":
		fmt.Fprintln(io.Out, versionText())
		return ExitOK
	default:
		fmt.Fprintf(io.Err, "Unknown command: %s\n\n", cmd)
		fmt.Fprint(io.Err, usageText())
		return ExitUsage
	}
}

func versionText() string {
	return app.VersionString() + ", " + buildTarget()
}

// runPersist verifies display-readiness and explains what (if anything) is
// needed.
func runPersist(ctx context.Context, a *app.App, io IO) int {
	r, err := a.Persist.Inspect(ctx)
	if err != nil {
		return reportError(io, "checking the machine", err)
	}
	for _, line := range r.Lines() {
		fmt.Fprintln(io.Out, line)
	}
	return ExitOK
}

// runClean performs the Clean & Finish procedure for the CLI: inspect,
// construct the cleanup plan, show both, ask for one deliberate
// confirmation, and only then execute. Nothing is modified before the
// confirmation; the same handover.Controller enforces the sequence for the
// TUI and the fallback menu.
func runClean(ctx context.Context, a *app.App, io IO, args []string) int {
	dryRun := false
	assumeYes := false

	for _, arg := range args {
		switch arg {
		case "--dry-run":
			dryRun = true
		case "--yes":
			assumeYes = true
		case "--help", "-h":
			fmt.Fprint(io.Out, helpText(a))
			return ExitOK
		default:
			return usageError(io, fmt.Sprintf("unknown option for clean: %s", arg))
		}
	}

	// Inspection and plan construction. Both are read-only.
	session, err := a.Handover.Begin(ctx)
	if err != nil {
		return reportError(io, "inspecting the machine before the handover", err)
	}
	fmt.Fprintln(io.Out, "Clean & Finish - inspection")
	fmt.Fprintln(io.Out, "==========================")
	for _, line := range session.Preflight.Explain() {
		fmt.Fprintln(io.Out, line)
	}
	if session.Plan != nil {
		fmt.Fprintln(io.Out)
		for _, line := range session.Plan.SummaryLines() {
			fmt.Fprintln(io.Out, line)
		}
	}

	if dryRun {
		fmt.Fprintln(io.Out)
		switch session.Preflight.Verdict {
		case handover.VerdictReady:
			fmt.Fprintln(io.Out, "Dry run: no changes were made. A handover appears possible; run \"winprep clean\" to perform it.")
		case handover.VerdictAttention:
			fmt.Fprintln(io.Out, "Dry run: no changes were made. A handover appears possible after the items above are addressed (or acknowledged).")
		default:
			fmt.Fprintln(io.Out, "Dry run: no changes were made. A handover is NOT possible until the issues above are resolved.")
		}
		return ExitOK
	}

	// A blocked machine stops here, regardless of --yes.
	if session.Preflight.Verdict == handover.VerdictBlocked {
		fmt.Fprintln(io.Out)
		fmt.Fprintln(io.Out, "The handover cannot proceed. Resolve the issues above and run \"winprep clean\" again.")
		return ExitFail
	}

	// One deliberate confirmation. Cancelling leaves the machine untouched.
	fmt.Fprintln(io.Out)
	fmt.Fprintln(io.Out, handover.ConfirmationWarning())
	if !assumeYes {
		if !io.Interactive {
			fmt.Fprintln(io.Out, "This command needs confirmation. Read the plan and warnings above, then run:")
			fmt.Fprintln(io.Out, "  winprep clean --yes")
			fmt.Fprintln(io.Out, "The safety checks above still apply; --yes does not bypass them or expand the plan.")
			return ExitFail
		}
		answer, ok := prompt(io, "Proceed with the cleanup and handover? [y/N] ")
		if !ok || !isAffirmative(answer) {
			fmt.Fprintln(io.Out, "Cancelled. Nothing was changed.")
			return ExitFail
		}
	}
	session.Approve()

	// The approved sequence: revalidate, clean, verify, reseal, report.
	fmt.Fprintln(io.Out)
	fmt.Fprintln(io.Out, "Removing the approved demonstration data, then resealing Windows...")
	res, err := session.Execute(ctx)
	if err != nil {
		return reportError(io, "running the handover", err)
	}
	fmt.Fprintln(io.Out)
	for _, line := range res.SummaryLines() {
		fmt.Fprintln(io.Out, line)
	}
	if res.Succeeded() {
		return ExitOK
	}
	return ExitFail
}

func isAffirmative(s string) bool {
	s = strings.TrimSpace(strings.ToLower(s))
	return s == "y" || s == "yes"
}

func prompt(io IO, question string) (string, bool) {
	if io.In == nil {
		return "", false
	}
	fmt.Fprint(io.Out, question)
	var line string
	if _, err := fmt.Fscanln(io.In, &line); err != nil {
		// An empty line (plain Enter) means the default, which is "no".
		return "", true
	}
	return line, true
}

func reportError(io IO, what string, err error) int {
	fmt.Fprintf(io.Err, "Error while %s:\n  %v\n", what, err)
	if isUnsupported(err) {
		fmt.Fprintln(io.Err, "WinPrep supports Windows 10 and 11 (x64) only.")
	}
	return ExitFail
}

func isUnsupported(err error) bool {
	return err != nil && strings.Contains(err.Error(), platform.ErrUnsupportedPlatform.Error())
}

func usageError(io IO, msg string) int {
	fmt.Fprintf(io.Err, "%s\n\n", msg)
	fmt.Fprint(io.Err, usageText())
	return ExitUsage
}
