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
	case "status":
		return runStatus(ctx, a, io)
	case "persist":
		return runPersist(ctx, a, io)
	case "check":
		return runCheck(ctx, a, io, rest)
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

// runStatus prints the read-only system summary.
func runStatus(ctx context.Context, a *app.App, io IO) int {
	r, err := a.Status.Inspect(ctx)
	if err != nil {
		return reportError(io, "reading the system status", err)
	}
	for _, line := range r.Lines() {
		fmt.Fprintln(io.Out, line)
	}
	return ExitOK
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

// runCheck performs the read-only diagnostics.
func runCheck(ctx context.Context, a *app.App, io IO, args []string) int {
	exportPath := ""
	batteryPath := ""
	wantBattery := false

	i := 0
	for i < len(args) {
		arg := args[i]
		switch arg {
		case "--export":
			if i+1 >= len(args) || strings.HasPrefix(args[i+1], "-") {
				return usageError(io, "--export needs a file path, for example: winprep check --export C:\\Users\\Public\\report.txt")
			}
			i++
			exportPath = args[i]
		case "--battery-report":
			wantBattery = true
			// The path is optional; a following non-flag token is the path.
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				i++
				batteryPath = args[i]
			}
		case "--help", "-h":
			fmt.Fprint(io.Out, helpText(a))
			return ExitOK
		default:
			return usageError(io, fmt.Sprintf("unknown option for check: %s", arg))
		}
		i++
	}

	report, err := a.Diag.Inspect(ctx)
	if err != nil {
		return reportError(io, "running diagnostics", err)
	}
	fmt.Fprint(io.Out, report.Text())

	if exportPath != "" {
		if err := writeTextFile(exportPath, report.Text()); err != nil {
			return reportError(io, "saving the report", err)
		}
		fmt.Fprintf(io.Out, "\nReport saved to: %s\n", exportPath)
	}

	if wantBattery {
		path, err := a.Diag.BatteryReport(ctx, batteryPath)
		if err != nil {
			return reportError(io, "generating the battery report", err)
		}
		fmt.Fprintf(io.Out, "\nBattery report saved to: %s\n", path)
	}
	return ExitOK
}

// runClean implements the seven-step handover procedure for the CLI.
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

	// Steps 1-3: inspect, evaluate, report.
	result, err := a.Handover.Inspect(ctx)
	if err != nil {
		return reportError(io, "inspecting the machine before the handover", err)
	}
	fmt.Fprintln(io.Out, "Handover inspection")
	fmt.Fprintln(io.Out, "===================")
	for _, line := range result.Explain() {
		fmt.Fprintln(io.Out, line)
	}

	if dryRun {
		fmt.Fprintln(io.Out)
		switch result.Verdict {
		case 0: // VerdictReady
			fmt.Fprintln(io.Out, "Dry run: no changes were made. A handover appears possible; run \"winprep clean\" to perform it.")
		case 1: // VerdictAttention
			fmt.Fprintln(io.Out, "Dry run: no changes were made. A handover appears possible after the issues above are addressed (or acknowledged).")
		default:
			fmt.Fprintln(io.Out, "Dry run: no changes were made. A handover is NOT possible until the issues above are resolved.")
		}
		return ExitOK
	}

	// A blocked machine stops here, regardless of --yes.
	if result.Verdict == 2 { // VerdictBlocked
		fmt.Fprintln(io.Out)
		fmt.Fprintln(io.Out, "The handover cannot proceed. Resolve the issues above and run \"winprep clean\" again.")
		return ExitFail
	}

	// Step 4: clean WinPrep's own artifacts.
	fmt.Fprintln(io.Out)
	fmt.Fprintln(io.Out, "Removing WinPrep's own artifacts...")
	cleanup, err := a.Handover.CleanupArtifacts(ctx)
	if err != nil {
		fmt.Fprintln(io.Out, "Cleanup did not complete:", err)
		fmt.Fprintln(io.Out, "The reseal was NOT started. Resolve the cleanup problem and run \"winprep clean\" again.")
		return ExitFail
	}
	if len(cleanup) == 0 {
		fmt.Fprintln(io.Out, "Nothing to remove; no WinPrep artifacts were recorded.")
	} else {
		for _, c := range cleanup {
			fmt.Fprintf(io.Out, "  removed: %s\n", c.Entry.Path)
		}
	}

	// Step 5: deliberate confirmation.
	fmt.Fprintln(io.Out)
	fmt.Fprintln(io.Out, warningBlock())
	if !assumeYes {
		if !io.Interactive {
			fmt.Fprintln(io.Out, "This command needs confirmation. Read the warnings above, then run:")
			fmt.Fprintln(io.Out, "  winprep clean --yes")
			fmt.Fprintln(io.Out, "The safety checks above still apply; --yes does not bypass them.")
			return ExitFail
		}
		answer, ok := prompt(io, "Proceed with the handover? [y/N] ")
		if !ok || !isAffirmative(answer) {
			fmt.Fprintln(io.Out, "Cancelled. Nothing further was changed; the laptop was NOT prepared for handover.")
			return ExitFail
		}
	}

	// Step 6: reseal.
	fmt.Fprintln(io.Out)
	fmt.Fprintln(io.Out, "Starting Sysprep. Do not turn off the laptop while it runs.")
	res, resealErr := a.Handover.Reseal(ctx)
	if resealErr != nil {
		return reportError(io, "running the handover", resealErr)
	}

	// Step 7: report the real outcome.
	fmt.Fprintln(io.Out)
	fmt.Fprintln(io.Out, res.Outcome.String())
	fmt.Fprintln(io.Out, res.Detail)
	if res.Outcome.String() == "Sysprep could not start" || res.Outcome.String() == "Sysprep reported an error" {
		fmt.Fprintln(io.Out)
		fmt.Fprintln(io.Out, "Sysprep writes details to these logs:")
		for _, p := range handoverLogPaths() {
			fmt.Fprintln(io.Out, "  "+p)
		}
		fmt.Fprintln(io.Out, "Microsoft's Sysprep troubleshooting guide: https://learn.microsoft.com/en-us/troubleshoot/windows-client/setup-upgrade-and-drivers/sysprep-fails-remove-or-update-store-apps")
		return ExitFail
	}
	return ExitOK
}

func handoverLogPaths() []string {
	return []string{
		`%WINDIR%\System32\Sysprep\Panther\setupact.log`,
		`%WINDIR%\System32\Sysprep\Panther\setuperr.log`,
		`%WINDIR%\Panther\setupact.log`,
		`%WINDIR%\Panther\setuperr.log`,
	}
}

func warningBlock() string {
	return strings.Join([]string{
		"You are about to prepare this laptop for handover to a buyer.",
		"",
		"  - Windows will be resealed into first-time setup (OOBE) and the laptop is",
		"    expected to shut down when Sysprep finishes.",
		"  - The process can fail. If it does, WinPrep will report the error and point",
		"    you to the Sysprep logs; the laptop will not be prepared.",
		"  - Sysprep does NOT securely erase data. Files, accounts, and traces from",
		"    your preparation and customer testing remain on the disk. If the laptop",
		"    must be wiped, do that separately before selling it.",
		"  - After the handover, start the laptop once yourself and verify that",
		"    Windows first-time setup appears, before handing it to the buyer.",
	}, "\n")
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
