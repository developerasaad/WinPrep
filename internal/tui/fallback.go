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

// runFallbackHandover performs Clean & Finish with plain-text prompts,
// driving the same controller sequence as the CLI and the full-screen TUI:
// inspect, plan, one deliberate confirmation, execute, report. Nothing is
// modified before the confirmation.
func runFallbackHandover(ctx context.Context, a *app.App, w *bufio.Writer, reader *bufio.Reader) {
	session, err := a.Handover.Begin(ctx)
	if err != nil {
		reportFallbackError(w, "inspecting the machine before the handover", err)
		return
	}
	fmt.Fprintln(w, "Clean & Finish - inspection")
	fmt.Fprintln(w, "==========================")
	for _, l := range session.Preflight.Explain() {
		fmt.Fprintln(w, l)
	}
	if session.Plan != nil {
		fmt.Fprintln(w)
		for _, l := range session.Plan.SummaryLines() {
			fmt.Fprintln(w, l)
		}
	}
	fmt.Fprintln(w)

	if session.Preflight.Verdict == handover.VerdictBlocked {
		fmt.Fprintln(w, "The handover cannot proceed. Resolve the issues above and try again.")
		fmt.Fprintln(w)
		return
	}

	for _, l := range strings.Split(handover.ConfirmationWarning(), "\n") {
		fmt.Fprintln(w, l)
	}
	fmt.Fprintln(w)
	if !askYes(w, reader, "Proceed with the cleanup and handover? [y/N] ", false) {
		fmt.Fprintln(w, "Cancelled. Nothing was changed.")
		fmt.Fprintln(w)
		return
	}
	session.Approve()

	fmt.Fprintln(w, "Removing the approved demonstration data, then resealing Windows...")
	res, xerr := session.Execute(ctx)
	fmt.Fprintln(w)
	if xerr != nil {
		fmt.Fprintln(w, "Error: "+xerr.Error())
		fmt.Fprintln(w)
		return
	}
	for _, l := range res.SummaryLines() {
		fmt.Fprintln(w, l)
	}
	fmt.Fprintln(w)
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
