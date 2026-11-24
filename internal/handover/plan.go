package handover

import (
	"fmt"

	"github.com/developerasaad/winprep/internal/format"
)

// planKind selects how the executor acts on a target. It is deliberately
// unexported: presentation code receives the plan as data and can never
// construct a target of its own.
type planKind int

const (
	// kindDirContents removes everything inside a directory and keeps the
	// directory itself (Downloads, the demo profile's Temp folder).
	kindDirContents planKind = iota
	// kindPaths removes an explicit list of paths (browser cache and data
	// files inside a browser profile directory).
	kindPaths
	// kindDataDir removes the WinPrep data directory and everything in it,
	// including the managed-artifact manifest.
	kindDataDir
)

// PlanTarget is one approved removal on the cleanup plan.
type PlanTarget struct {
	kind  planKind
	Label string // e.g. "Downloads"
	Root  string // the location acted on, shown to the operator
	// Paths are the concrete paths for kindTargets (browser data files).
	// For kindDirContents the children are re-listed at execution time, so
	// files added to an approved location after the plan still belong to the
	// approved scope, which is the location, not a frozen file list.
	Paths []string
	// Items and Size summarize what was found at plan time.
	Items int
	Size  uint64
}

// CleanupPlan is the typed, read-only proposal of what "Clean & Finish" will
// remove. It is constructed before anything is modified and shown to the
// operator; only an explicit approval turns it into deletions.
type CleanupPlan struct {
	// DemoProfile is the profile root of the built-in Administrator account,
	// resolved from the registry by SID. It is the environment customers
	// used, and the only profile whose data the plan may touch.
	DemoProfile string

	// WinPrepTargets are items WinPrep owns by construction: its data
	// directory and the artifacts recorded in its manifest. Removing them
	// needs no separate authorization beyond the handover confirmation.
	WinPrepTargets []PlanTarget

	// DemoTargets are demonstration-data items inside the demo profile.
	DemoTargets []PlanTarget

	// Notes report things the operator should review that WinPrep will not
	// touch: redirected shell folders, non-empty Desktop or Documents, extra
	// browser profiles.
	Notes []string

	// Blockers are conditions that make a safe cleanup impossible right now
	// (a running browser, a tampered manifest, winprep.exe inside a removal
	// target). Any blocker stops the handover before anything is deleted.
	Blockers []string
}

// HasActions reports whether the plan would remove anything at all.
func (p *CleanupPlan) HasActions() bool {
	return len(p.WinPrepTargets)+len(p.DemoTargets) > 0
}

// IsBlocked reports whether a condition prevents the cleanup from proceeding.
func (p *CleanupPlan) IsBlocked() bool {
	return len(p.Blockers) > 0
}

// SummaryLines renders the plan as plain text, shared by the CLI, the
// fallback menu, and the TUI review screen.
func (p *CleanupPlan) SummaryLines() []string {
	var lines []string
	if p.DemoProfile != "" {
		lines = append(lines, "Demonstration profile: "+p.DemoProfile)
		lines = append(lines, "")
	}

	if len(p.WinPrepTargets) > 0 {
		lines = append(lines, "WinPrep's own files, removed by the cleanup:")
		for _, t := range p.WinPrepTargets {
			lines = append(lines, "  "+targetSummary(t))
		}
		lines = append(lines, "")
	}

	if len(p.DemoTargets) > 0 {
		lines = append(lines, "Demonstration data to remove:")
		for _, t := range p.DemoTargets {
			lines = append(lines, "  "+targetSummary(t))
		}
		lines = append(lines, "")
	}

	if len(p.Notes) > 0 {
		lines = append(lines, "For your review (WinPrep will not touch these):")
		for _, n := range p.Notes {
			lines = append(lines, "  - "+n)
		}
		lines = append(lines, "")
	}

	if len(p.Blockers) > 0 {
		lines = append(lines, "Cannot be cleaned automatically - resolve these first:")
		for _, b := range p.Blockers {
			lines = append(lines, "  - "+b)
		}
	}

	if !p.HasActions() {
		lines = append(lines, "No removable demonstration data was found.")
	}
	return lines
}

func targetSummary(t PlanTarget) string {
	if t.Items == 0 {
		return fmt.Sprintf("%s (empty) - %s", t.Label, t.Root)
	}
	return fmt.Sprintf("%s - %s (%d items, %s)", t.Label, t.Root, t.Items, format.Bytes(t.Size))
}

// TargetResult reports what happened to one plan target during execution.
type TargetResult struct {
	Label   string
	Removed int
	// Failed lists "path: error" entries for removals that did not succeed.
	Failed []string
}

// ExecutionResult reports the outcome of an approved cleanup run.
type ExecutionResult struct {
	TargetResults []TargetResult
	// Complete is true only when every removal succeeded and the
	// verification pass confirmed the required results. When it is false the
	// reseal must not start.
	Complete bool
}

// SummaryLines renders the execution outcome for the operator.
func (r ExecutionResult) SummaryLines() []string {
	var lines []string
	for _, tr := range r.TargetResults {
		if len(tr.Failed) == 0 {
			if tr.Removed == 0 {
				lines = append(lines, fmt.Sprintf("%s: nothing to remove.", tr.Label))
			} else {
				lines = append(lines, fmt.Sprintf("%s: removed %d items.", tr.Label, tr.Removed))
			}
			continue
		}
		lines = append(lines, fmt.Sprintf("%s: %d removed, %d FAILED:", tr.Label, tr.Removed, len(tr.Failed)))
		for _, f := range tr.Failed {
			lines = append(lines, "  "+f)
		}
	}
	if !r.Complete {
		lines = append(lines, "",
			"The cleanup did not complete. The reseal was NOT started.",
			"Resolve the failures above (closing the programs that hold the files open",
			"usually helps) and run the cleanup again; it safely retries what remains.")
	}
	return lines
}

// FailedTargets lists the labels of targets with failures, for tests and
// diagnostics.
func (r ExecutionResult) FailedTargets() []string {
	var out []string
	for _, tr := range r.TargetResults {
		if len(tr.Failed) > 0 {
			out = append(out, tr.Label)
		}
	}
	return out
}
