package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/developerasaad/winprep/internal/app"
	"github.com/developerasaad/winprep/internal/audit"
	"github.com/developerasaad/winprep/internal/handover"
)

// View renders the active screen. The alt screen is used so the operator's
// console is restored on exit.
func (m *Model) View() tea.View {
	v := tea.NewView(m.render())
	v.AltScreen = true
	return v
}

// render produces the full screen content as plain lines.
func (m *Model) render() string {
	var b strings.Builder
	switch m.screen {
	case screenMenu:
		b.WriteString(m.renderMenu())
	default:
		b.WriteString(m.renderScreen())
	}
	b.WriteString("\n")
	if m.notice != "" {
		b.WriteString("\n")
		b.WriteString(m.notice)
		b.WriteString("\n")
	}
	b.WriteString("\n")
	b.WriteString(styleRule.Render(strings.Repeat("-", m.ruleWidth())))
	b.WriteString("\n")
	b.WriteString(styleDim.Render(m.footerHint()))
	return b.String()
}

func (m *Model) ruleWidth() int {
	w := m.width
	if w <= 0 || w > contentWidth {
		w = contentWidth
	}
	if w < 20 {
		w = 20
	}
	return w
}

// renderMenu draws the main menu: title, fast readiness header, options.
func (m *Model) renderMenu() string {
	var b strings.Builder
	b.WriteString(styleTitle.Render("WinPrep"))
	b.WriteString(strings.Repeat(" ", maxInt(1, m.ruleWidth()-len("WinPrep")-len(app.Version))))
	b.WriteString(styleDim.Render("v" + app.Version))
	b.WriteString("\n")
	b.WriteString(styleDim.Render("Windows preparation"))
	b.WriteString("\n\n")

	if !m.headerShown {
		if m.loading {
			b.WriteString(styleDim.Render("Status: checking..."))
			b.WriteString("\n")
		}
	} else if m.headerErr != nil {
		b.WriteString(styleError.Render("Status: could not read (" + m.headerErr.Error() + ")"))
		b.WriteString("\n")
	} else {
		b.WriteString("Current mode: " + modeStyle(m.header.Audit.State).Render(m.header.Audit.State.Short()) + "\n")
		if m.header.ReadyForDemo {
			b.WriteString("Status: " + styleOK.Render("Ready for display") + "\n")
		} else {
			b.WriteString("Status: " + styleWarn.Render("Not ready for display") + "\n")
		}
	}
	b.WriteString("\n")

	for i, item := range menuItems {
		prefix := "  "
		label := item.label
		if i == m.cursor {
			prefix = "> "
			label = styleSelected.Render(label)
		} else {
			label = styleNormal.Render(label)
		}
		b.WriteString(prefix + label + "\n")
	}
	b.WriteString("\n")
	b.WriteString(styleDim.Render(wrapOne(menuItems[m.cursor].desc, m.ruleWidth())))
	return b.String()
}

// modeStyle colors the current-mode line by audit state. Confirmed Audit
// Mode is the only green state.
func modeStyle(s audit.State) lipgloss.Style {
	switch s {
	case audit.StateAuditMode:
		return styleOK
	case audit.StateUncertain, audit.StateUnknown:
		return styleWarn
	default:
		return styleNormal
	}
}

// renderScreen draws the titled, scrollable screens.
func (m *Model) renderScreen() string {
	var b strings.Builder
	b.WriteString(styleTitle.Render(m.screenTitle()))
	b.WriteString("\n\n")

	if m.loading {
		line := "Working: " + m.loadingWhat + "..."
		if m.loadingCancellable {
			line += " (ESC cancels)"
		}
		b.WriteString(styleDim.Render(line))
		return b.String()
	}

	lines := m.viewportLines()
	top, bottom := m.scroll, m.scroll+m.viewportHeight()
	if top > len(lines) {
		top = len(lines)
	}
	if bottom > len(lines) {
		bottom = len(lines)
	}
	for _, l := range lines[top:bottom] {
		b.WriteString(l)
		b.WriteString("\n")
	}
	if bottom < len(lines) {
		b.WriteString(styleDim.Render("... more below (DOWN to scroll) ...\n"))
	}

	// Screen-specific interactive rows below the viewport.
	switch m.screen {
	case screenHandover:
		switch m.phase {
		case hoConfirm:
			b.WriteString(m.renderConfirm())
		}
	}
	return b.String()
}

func (m *Model) screenTitle() string {
	switch m.screen {
	case screenHelp:
		return "Help"
	case screenPersist:
		return "Prepare for Display"
	case screenHandover:
		switch m.phase {
		case hoInspect, hoReview:
			return "Clean & Finish - inspection"
		case hoConfirm:
			return "Clean & Finish - confirm"
		case hoReseal:
			return "Clean & Finish - finishing"
		default:
			return "Clean & Finish - result"
		}
	}
	return "WinPrep"
}

// renderConfirm draws the deliberate-confirmation choice. The safe option is
// selected by default; "Proceed" must be chosen on purpose.
func (m *Model) renderConfirm() string {
	var b strings.Builder
	b.WriteString("\n")
	for _, l := range wrap(handoverWarning(), m.wrapWidth()) {
		b.WriteString(styleWarn.Render(l))
		b.WriteString("\n")
	}
	if m.preflight.Verdict == handover.VerdictAttention {
		b.WriteString("\n")
		b.WriteString(styleWarn.Render("There are items marked \"Requires attention\" in the inspection above."))
		b.WriteString("\n")
		b.WriteString(styleWarn.Render("Continuing acknowledges that you reviewed them."))
		b.WriteString("\n")
	}
	b.WriteString("\n")
	choices := []string{"Cancel (nothing happens)", "Proceed: clean up and prepare for the buyer"}
	for i, c := range choices {
		if i == m.confirmChoice {
			b.WriteString("> " + styleSelected.Render(c) + "\n")
		} else {
			b.WriteString("  " + styleNormal.Render(c) + "\n")
		}
	}
	return b.String()
}

// footerHint names the controls available on the active screen.
func (m *Model) footerHint() string {
	if m.loading {
		if m.loadingCancellable {
			return "ESC Cancel"
		}
		return "Please wait"
	}
	switch m.screen {
	case screenMenu:
		return "UP/DOWN Move   ENTER Select   CTRL+C Exit"
	case screenHandover:
		switch m.phase {
		case hoReview:
			if m.preflight.Verdict == handover.VerdictBlocked {
				return "UP/DOWN Scroll   ESC Back"
			}
			return "ENTER Continue   UP/DOWN Scroll   ESC Back"
		case hoConfirm:
			return "LEFT/RIGHT Choose   ENTER Select   Y/N Shortcut"
		case hoReseal:
			return "Sysprep is running - do not turn off the laptop"
		default:
			return "UP/DOWN Scroll   ESC Back"
		}
	default:
		return "UP/DOWN Scroll   ESC Back"
	}
}

// handoverWarning is the confirmation text shown before the handover runs.
func handoverWarning() string {
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

// renderPreflight formats the preflight checks for the TUI.
func renderPreflight(r handover.PreflightResult, err error) []string {
	if err != nil {
		return []string{"The inspection could not complete:", "", "  " + err.Error(), "",
			"Nothing was changed. Try again; if it keeps failing, report this as a bug."}
	}
	var lines []string
	lines = append(lines, "Overall: "+verdictStyle(r.Verdict.String()).Render(r.Verdict.String()))
	lines = append(lines, "")
	for _, c := range r.Checks {
		lines = append(lines, c.Title+": "+severityStyle(c.Severity.String()).Render(c.Severity.String()))
		if c.Detail != "" {
			lines = append(lines, "  "+c.Detail)
		}
		if c.NextStep != "" {
			lines = append(lines, "  Next step: "+c.NextStep)
		}
		lines = append(lines, "")
	}
	switch r.Verdict {
	case handover.VerdictReady:
		lines = append(lines, "A handover appears possible.")
	case handover.VerdictAttention:
		lines = append(lines, "A handover appears possible after the items above are reviewed.")
	default:
		lines = append(lines, "A handover is NOT possible until the issues above are resolved.")
	}
	return lines
}

// renderOutcome formats the post-reseal result, distinguishing cleanup
// failure, Sysprep launch failure, Sysprep error, and initiation.
func renderOutcome(cleanup []handover.CleanupResult, cleanupErr error, reseal handover.ResealResult, resealErr error) []string {
	var lines []string
	if cleanupErr != nil {
		lines = append(lines, styleError.Render("Cleaning up WinPrep's artifacts failed:"))
		lines = append(lines, "  "+cleanupErr.Error())
		lines = append(lines, "",
			"The reseal was NOT started. Resolve the cleanup problem and run",
			"\"Clean & Finish\" again.")
		return lines
	}
	if len(cleanup) == 0 {
		lines = append(lines, "WinPrep artifacts: nothing to remove.")
	} else {
		lines = append(lines, "WinPrep artifacts removed:")
		for _, c := range cleanup {
			if c.Err != "" {
				lines = append(lines, "  failed: "+c.Entry.Path+" ("+c.Err+")")
			} else {
				lines = append(lines, "  removed: "+c.Entry.Path)
			}
		}
	}
	lines = append(lines, "")

	if resealErr != nil {
		lines = append(lines, styleError.Render("The handover failed:"))
		lines = append(lines, "  "+resealErr.Error())
		lines = append(lines, "", "Sysprep writes details to these logs:")
		lines = append(lines, handover.PantherLogPaths()...)
		return lines
	}

	switch reseal.Outcome {
	case handover.ResealInitiated:
		lines = append(lines, styleOK.Render(reseal.Outcome.String()))
		lines = append(lines, reseal.Detail)
		lines = append(lines, "",
			"WinPrep's part is done. The claims of a successful handover still need your",
			"own verification: switch the laptop on once and confirm that Windows",
			"first-time setup appears before selling it.")
	default:
		lines = append(lines, styleError.Render(reseal.Outcome.String()))
		lines = append(lines, reseal.Detail)
		lines = append(lines, "", "Sysprep writes details to these logs:")
		lines = append(lines, handover.PantherLogPaths()...)
		lines = append(lines, "",
			"Microsoft's Sysprep troubleshooting guide:",
			"https://learn.microsoft.com/en-us/troubleshoot/windows-client/setup-upgrade-and-drivers/sysprep-fails-remove-or-update-store-apps")
	}
	return lines
}

// helpText is the help screen. It mirrors the CLI help where practical.
func helpText() string {
	return strings.Join([]string{
		"WinPrep prepares Windows laptops for resale.",
		"",
		"The shop workflow:",
		"",
		"  1. Install Windows and enter Audit Mode before finishing first-time",
		"     setup (press Ctrl+Shift+F3 on the setup screen).",
		"  2. Install drivers and get the laptop ready; use \"Prepare for Display\"",
		"     to verify it stays demonstrable. Audit Mode survives restarts and",
		"     shutdowns on its own - WinPrep adds no persistence tricks.",
		"  3. Let customers test the laptop freely.",
		"  4. Before the sale, use \"Clean & Finish\" to clean the approved",
		"     demonstration data and reseal Windows into first-time setup. The",
		"     laptop shuts down; the buyer completes Windows setup.",
		"",
		"What WinPrep does NOT do:",
		"",
		"  - It is not a factory reset and not a secure erase. Resealing Windows",
		"    with Sysprep leaves existing files and accounts on the disk. Wipe",
		"    the disk separately if the data must be gone.",
		"  - It does not remove user accounts and does not install or update",
		"    drivers.",
		"  - It never disables Windows security features.",
		"",
		"Security note:",
		"",
		"  Audit Mode signs you in as the built-in Administrator with full",
		"  rights. It is a preparation environment, not a safe public kiosk -",
		"  anything a customer runs on the machine runs with administrator",
		"  privileges. See docs/safety.md before relying on this workflow.",
		"",
		"Controls:",
		"",
		"  UP/DOWN   Move the selection (or scroll a report)",
		"  ENTER     Select the highlighted option",
		"  ESC       Back out; cancels a running check",
		"  Y/N       Confirm or reject at warnings",
		"  CTRL+C    Exit (blocked while Sysprep runs)",
		"",
		"The same operations are available as commands (winprep persist,",
		"winprep clean) for scripted use.",
	}, "\n")
}

// wrapLines wraps each line of a line slice.
func wrapLines(lines []string, width int) []string {
	var out []string
	for _, l := range lines {
		out = append(out, wrap(l, width)...)
	}
	return out
}

// wrapOne wraps a single string into one joined line.
func wrapOne(s string, width int) string {
	return strings.Join(wrap(s, width), "\n")
}
