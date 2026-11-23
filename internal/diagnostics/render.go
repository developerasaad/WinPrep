package diagnostics

import (
	"fmt"
	"strings"
)

// Text renders the full report as plain, ASCII-only text. It is used for the
// CLI output, the fallback menu, and exported report files. No personally
// identifying information (user names, serial numbers) is included.
func (r Report) Text() string {
	var b strings.Builder
	b.WriteString("WinPrep diagnostic report\n")
	b.WriteString("This report reflects what Windows reported at the time of the check.\n")
	b.WriteString("It is informational and does not replace a physical inspection.\n")
	for i, s := range r.Sections {
		if i > 0 {
			b.WriteString("\n")
		}
		fmt.Fprintf(&b, "== %s\n", s.Title)
		for _, l := range s.Lines {
			switch {
			case l.Available && l.Value != "":
				fmt.Fprintf(&b, "  %-20s %s\n", l.Label+":", l.Value)
			case l.Note != "":
				fmt.Fprintf(&b, "  %-20s %s\n", l.Label+":", l.Note)
			default:
				fmt.Fprintf(&b, "  %-20s (not available)\n", l.Label+":")
			}
		}
	}
	return b.String()
}

// AllAvailable reports whether every line in the report carried a value. It
// drives the "some information was unavailable" hint in the UI.
func (r Report) AllAvailable() bool {
	for _, s := range r.Sections {
		for _, l := range s.Lines {
			if !l.Available && (l.Value != "" || l.Note == "") {
				return false
			}
		}
	}
	return true
}
