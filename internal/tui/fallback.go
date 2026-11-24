package tui

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/developerasaad/winprep/internal/app"
	"github.com/developerasaad/winprep/internal/handover"
)

// RunFallback drives a plain numbered menu for consoles that cannot host the
// full-screen interface but are still usable interactively (for example a
// classic console with limited escape-sequence support). It uses ordinary
// line input and plain text output only - no colors, no cursor addressing.
//
// It runs the same App services as the TUI and CLI.
func RunFallback(ctx context.Context, a *app.App, in io.Reader, out io.Writer) int {
	w := bufio.NewWriter(out)
	defer w.Flush()

	fmt.Fprintln(w, "WinPrep - simplified menu (this console does not support the full interface)")
	fmt.Fprintln(w)

	reader := bufio.NewReader(in)
	for {
		fmt.Fprintln(w, "  1  Prepare for Display")
		fmt.Fprintln(w, "  2  Clean & Finish")
		fmt.Fprintln(w, "  3  Help")
		fmt.Fprintln(w, "  4  Exit")
		fmt.Fprint(w, "Choose 1-4: ")
		w.Flush()

		line, err := reader.ReadString('\n')
		if err != nil {
			// EOF without input ends the session quietly.
			fmt.Fprintln(w)
			return 0
		}
		choice, convErr := strconv.Atoi(strings.TrimSpace(line))
		if convErr != nil || choice < 1 || choice > 4 {
			fmt.Fprintln(w, "Please enter a number from 1 to 4.")
			fmt.Fprintln(w)
			continue
		}

		switch choice {
		case 1:
			runFallbackAction(w, reader, "checking this laptop", func() ([]string, error) {
				r, err := a.Persist.Inspect(ctx)
				return r.Lines(), err
			})
		case 2:
			runFallbackHandover(ctx, a, w, reader)
		case 3:
			runFallbackAction(w, reader, "showing help", func() ([]string, error) {
				return strings.Split(helpText(), "\n"), nil
			})
		case 4:
			return 0
		}
	}
}

// runFallbackAction runs one read-only service call, prints its lines, and
// waits for ENTER so the output stays on screen.
func runFallbackAction(w *bufio.Writer, reader *bufio.Reader, what string, fn func() ([]string, error)) {
	lines, err := fn()
	if err != nil {
		reportFallbackError(w, what, err)
		return
	}
	for _, l := range lines {
		fmt.Fprintln(w, l)
	}
	fmt.Fprint(w, "Press ENTER to return to the menu. ")
	w.Flush()
	_, _ = reader.ReadString('\n')
	fmt.Fprintln(w)
}

// runFallbackHandover performs the handover steps with plain-text prompts.
func runFallbackHandover(ctx context.Context, a *app.App, w *bufio.Writer, reader *bufio.Reader) {
	result, err := a.Handover.Inspect(ctx)
	if err != nil {
		reportFallbackError(w, "inspecting the machine before the handover", err)
		return
	}
	fmt.Fprintln(w, "Handover inspection")
	fmt.Fprintln(w, "===================")
	for _, l := range result.Explain() {
		fmt.Fprintln(w, l)
	}
	fmt.Fprintln(w)

	switch result.Verdict {
	case handover.VerdictBlocked:
		fmt.Fprintln(w, "The handover cannot proceed. Resolve the issues above and try again.")
		fmt.Fprintln(w)
		return
	case handover.VerdictReady:
		// Nothing extra to say.
	default:
		if !askYes(w, reader, "There are items requiring attention. Review them above, then continue? [y/N] ", false) {
			fmt.Fprintln(w, "Cancelled. Nothing was changed.")
			fmt.Fprintln(w)
			return
		}
	}

	fmt.Fprintln(w, "Removing WinPrep's own artifacts...")
	cleanup, cerr := a.Handover.CleanupArtifacts(ctx)
	if cerr != nil {
		fmt.Fprintln(w, "Cleanup did not complete: "+cerr.Error())
		fmt.Fprintln(w, "The reseal was NOT started. Resolve the cleanup problem and try again.")
		fmt.Fprintln(w)
		return
	}
	if len(cleanup) == 0 {
		fmt.Fprintln(w, "Nothing to remove; no WinPrep artifacts were recorded.")
	} else {
		for _, c := range cleanup {
			fmt.Fprintln(w, "  removed: "+c.Entry.Path)
		}
	}
	fmt.Fprintln(w)

	if !askYes(w, reader, proceedQuestion(), false) {
		fmt.Fprintln(w, "Cancelled. Nothing further was changed; the laptop was NOT prepared for handover.")
		fmt.Fprintln(w)
		return
	}

	fmt.Fprintln(w, "Starting Sysprep. Do not turn off the laptop while it runs.")
	res, rerr := a.Handover.Reseal(ctx)
	fmt.Fprintln(w)
	fmt.Fprintln(w, res.Outcome.String())
	if res.Detail != "" {
		fmt.Fprintln(w, res.Detail)
	}
	if rerr != nil {
		fmt.Fprintln(w, "Error: "+rerr.Error())
	}
	if res.Outcome != handover.ResealInitiated {
		fmt.Fprintln(w)
		fmt.Fprintln(w, "Sysprep writes details to these logs:")
		for _, p := range handover.PantherLogPaths() {
			fmt.Fprintln(w, "  "+p)
		}
	}
	fmt.Fprintln(w)
}

// proceedQuestion is the confirmation text, shared verbatim with the CLI so
// both interfaces warn identically.
func proceedQuestion() string {
	return strings.Join([]string{
		"You are about to prepare this laptop for handover to a buyer.",
		"  - Windows will be resealed into first-time setup (OOBE) and the laptop is",
		"    expected to shut down when Sysprep finishes.",
		"  - The process can fail. If it does, WinPrep will report the error and point",
		"    you to the Sysprep logs; the laptop will not be prepared.",
		"  - Sysprep does NOT securely erase data. Files, accounts, and traces from",
		"    your preparation and customer testing remain on the disk. If the laptop",
		"    must be wiped, do that separately before selling it.",
		"  - After the handover, start the laptop once yourself and verify that",
		"    Windows first-time setup appears, before handing it to the buyer.",
		"",
		"Proceed with the handover? [y/N] ",
	}, "\n")
}

// askYes asks a yes/no question. defaultYes selects the answer for an empty
// ENTER; the safe default for dangerous actions is false.
func askYes(w *bufio.Writer, reader *bufio.Reader, question string, defaultYes bool) bool {
	fmt.Fprint(w, question)
	w.Flush()
	line, err := reader.ReadString('\n')
	if err != nil && line == "" {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes":
		return true
	case "n", "no":
		return false
	default:
		return defaultYes
	}
}

func reportFallbackError(w *bufio.Writer, what string, err error) {
	fmt.Fprintf(w, "Error while %s:\n  %v\n\n", what, err)
	w.Flush()
}
