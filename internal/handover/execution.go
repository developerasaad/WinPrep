package handover

import (
	"os"
	"path/filepath"
)

// ExecutePlan performs the approved removals. It is only ever called with a
// plan the operator explicitly confirmed. Every concrete path is revalidated
// immediately before it is deleted, and reparse points (symlinks, junctions)
// are removed as links without ever following them, so a redirection swapped
// in after planning cannot send a deletion outside the approved scope.
//
// Execution is deliberately retryable: each target is independent, failures
// are recorded rather than aborting the run, and the result states plainly
// whether the cleanup completed. An incomplete cleanup must block the
// reseal; that decision belongs to the caller.
func ExecutePlan(plan CleanupPlan) ExecutionResult {
	res := ExecutionResult{Complete: true}
	for _, group := range [][]PlanTarget{plan.WinPrepTargets, plan.DemoTargets} {
		for _, t := range group {
			res.TargetResults = append(res.TargetResults, executeTarget(t))
		}
	}
	for _, tr := range res.TargetResults {
		if len(tr.Failed) > 0 {
			res.Complete = false
		}
	}
	if res.Complete && !verifyPlan(plan) {
		res.Complete = false
	}
	return res
}

// executeTarget removes one plan target and reports what happened.
func executeTarget(t PlanTarget) TargetResult {
	tr := TargetResult{Label: t.Label}

	remove := func(path string) {
		if err := removePath(path); err != nil {
			tr.Failed = append(tr.Failed, path+": "+err.Error())
			return
		}
		tr.Removed++
	}

	switch t.kind {
	case kindDataDir:
		// The data directory contains the manifest itself, so the explicit
		// paths for this target come from a fresh directory listing.
		if entries, err := os.ReadDir(t.Root); err == nil {
			for _, e := range entries {
				remove(filepath.Join(t.Root, e.Name()))
			}
		} else if !os.IsNotExist(err) {
			tr.Failed = append(tr.Failed, t.Root+": "+err.Error())
			return tr
		}
		if err := os.Remove(t.Root); err != nil && !os.IsNotExist(err) {
			tr.Failed = append(tr.Failed, t.Root+": "+err.Error())
		} else {
			tr.Removed++
		}

	case kindDirContents:
		entries, err := os.ReadDir(t.Root)
		if err != nil {
			if os.IsNotExist(err) {
				return tr // nothing there; nothing to remove
			}
			tr.Failed = append(tr.Failed, t.Root+": "+err.Error())
			return tr
		}
		for _, e := range entries {
			child := filepath.Join(t.Root, e.Name())
			// Containment is revalidated per child: the executor only ever
			// removes entries of the approved directory itself.
			if !containsPath(t.Root, child) {
				tr.Failed = append(tr.Failed, child+": outside the approved cleanup scope; refused")
				continue
			}
			remove(child)
		}

	case kindPaths:
		for _, p := range t.Paths {
			if !containsPath(t.Root, p) {
				tr.Failed = append(tr.Failed, p+": outside the approved cleanup scope; refused")
				continue
			}
			remove(p)
		}
	}
	return tr
}

// removePath deletes one path. Reparse points are unlinked, never followed;
// directories are emptied bottom-up. A path that is already absent counts as
// removed, which keeps a retry after an interruption idempotent.
func removePath(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		// A symlink or junction: delete the link only. The executor never
		// follows it, so the link target is untouched wherever it points.
		return os.Remove(path)
	}
	if !info.IsDir() {
		return os.Remove(path)
	}
	return removeTree(path)
}

// removeTree recursively deletes a real directory (not a reparse point).
func removeTree(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if err := removePath(filepath.Join(dir, e.Name())); err != nil {
			return err
		}
	}
	return os.Remove(dir)
}

// verifyPlan checks the required postconditions: contents targets are empty,
// explicit paths and the data directory are gone. It reports whether the
// cleanup achieved what the plan promised.
func verifyPlan(plan CleanupPlan) bool {
	ok := true
	for _, group := range [][]PlanTarget{plan.WinPrepTargets, plan.DemoTargets} {
		for _, t := range group {
			switch t.kind {
			case kindDataDir:
				if _, err := os.Lstat(t.Root); !os.IsNotExist(err) {
					ok = false
				}
			case kindPaths:
				for _, p := range t.Paths {
					if _, err := os.Lstat(p); !os.IsNotExist(err) {
						ok = false
					}
				}
			case kindDirContents:
				entries, err := os.ReadDir(t.Root)
				if err != nil || len(entries) > 0 {
					ok = false
				}
			}
		}
	}
	return ok
}
