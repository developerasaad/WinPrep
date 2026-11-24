// Package tui implements WinPrep's interactive terminal interface using
// Bubble Tea. It renders the same App services the CLI uses; no business
// logic lives here.
//
// Design constraints:
//
//   - ASCII-only drawing (">" cursor, "-" markers) so the interface renders
//     in every Windows console, including classic conhost with a raster font.
//   - Restrained color: color is used to classify severity and state, never
//     for decoration. All styling degrades gracefully when the terminal
//     reports no color support.
//   - The menu appears immediately; anything slower than a registry read is
//     loaded asynchronously with a visible "working" line and ESC to cancel.
package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
)

// contentWidth is the maximum rendered width. Reports are wrapped to it so
// long values stay readable on ordinary laptop screens.
const contentWidth = 76

var (
	styleTitle = lipgloss.NewStyle().Bold(true)
	styleDim   = lipgloss.NewStyle().Faint(true)

	styleSelected = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("15"))
	styleNormal   = lipgloss.NewStyle()

	styleOK    = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	styleWarn  = lipgloss.NewStyle().Foreground(lipgloss.Color("11"))
	styleError = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))

	styleRule = lipgloss.NewStyle().Faint(true)
)

// severityStyle maps a handover severity to its color style.
func severityStyle(s string) lipgloss.Style {
	switch s {
	case "Passed":
		return styleOK
	case "Requires attention":
		return styleWarn
	default: // "Blocked", "Unknown"
		return styleError
	}
}

// verdictStyle maps a handover verdict to its color style.
func verdictStyle(v string) lipgloss.Style {
	switch v {
	case "Ready":
		return styleOK
	case "Requires attention":
		return styleWarn
	default:
		return styleError
	}
}

// wrap breaks text into lines no longer than width characters, preferring
// spaces and hard-breaking tokens that do not fit. It preserves explicit line
// breaks and keeps runs of spaces (registry paths) intact where possible.
func wrap(s string, width int) []string {
	if width < 8 {
		width = 8
	}
	var out []string
	for _, para := range strings.Split(s, "\n") {
		if para == "" {
			out = append(out, "")
			continue
		}
		// strings.Fields loses the indentation of wrapped continuation lines,
		// which is fine for our reports: continuation starts flush left.
		words := strings.Fields(para)
		if len(words) == 0 {
			out = append(out, "")
			continue
		}
		line := words[0]
		for len(line) > width { // hard-break a very long first token
			out = append(out, line[:width])
			line = line[width:]
		}
		for _, w := range words[1:] {
			if len(line)+1+len(w) <= width {
				line += " " + w
			} else {
				out = append(out, line)
				for len(w) > width { // hard-break very long tokens (SIDs, paths)
					out = append(out, w[:width])
					w = w[width:]
				}
				line = w
			}
		}
		out = append(out, line)
	}
	return out
}

// plain strips ANSI escape sequences, used when measuring styled strings.
func plain(s string) string {
	var b strings.Builder
	inEscape := false
	for _, r := range s {
		switch {
		case inEscape:
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
				inEscape = false
			}
		case r == 0x1b:
			inEscape = true
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}
