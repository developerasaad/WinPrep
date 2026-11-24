package handover

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func makeContentsPlan(t *testing.T, files ...string) CleanupPlan {
	t.Helper()
	root := t.TempDir()
	for _, rel := range files {
		full := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return CleanupPlan{
		DemoTargets: []PlanTarget{{
			kind:  kindDirContents,
			Label: "Downloads",
			Root:  root,
		}},
	}
}

func TestExecuteRemovesContentsKeepsFolder(t *testing.T) {
	plan := makeContentsPlan(t, "a.txt", "b.txt", filepath.Join("sub", "c.txt"))
	res := ExecutePlan(plan)
	if !res.Complete {
		t.Fatalf("execution must complete: %v", res.TargetResults)
	}
	root := plan.DemoTargets[0].Root
	if _, err := os.Lstat(root); err != nil {
		t.Fatalf("the folder itself must survive: %v", err)
	}
	entries, _ := os.ReadDir(root)
	if len(entries) != 0 {
		t.Errorf("folder must be empty, still holds %d entries", len(entries))
	}
	if tr := res.TargetResults[0]; tr.Removed != 3 || len(tr.Failed) != 0 {
		t.Errorf("target result = %+v, want 3 removed, no failures", tr)
	}
}

func TestExecuteRemovesExplicitPathsOnly(t *testing.T) {
	root := t.TempDir()
	edge := filepath.Join(root, "Default")
	cache := filepath.Join(edge, "Cache")
	history := filepath.Join(edge, "History")
	networkCookies := filepath.Join(edge, "Network", "Cookies")
	for _, dir := range []string{cache, filepath.Join(edge, "Network")} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, f := range []string{filepath.Join(cache, "f"), history, networkCookies, filepath.Join(edge, "Preferences")} {
		if err := os.WriteFile(f, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	plan := CleanupPlan{DemoTargets: []PlanTarget{{
		kind:  kindPaths,
		Label: "Edge browsing data",
		Root:  edge,
		Paths: []string{cache, history, networkCookies},
	}}}

	res := ExecutePlan(plan)
	if !res.Complete {
		t.Fatalf("execution must complete: %v", res.TargetResults)
	}
	for _, p := range plan.DemoTargets[0].Paths {
		if _, err := os.Lstat(p); !os.IsNotExist(err) {
			t.Errorf("%s should be gone", p)
		}
	}
	if _, err := os.Lstat(filepath.Join(edge, "Preferences")); err != nil {
		t.Errorf("untouched files must survive: %v", err)
	}
}

func TestExecuteRemovesDataDirEntirely(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "managed-artifacts.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	plan := CleanupPlan{WinPrepTargets: []PlanTarget{{
		kind:  kindDataDir,
		Label: "WinPrep data directory",
		Root:  dir,
	}}}

	res := ExecutePlan(plan)
	if !res.Complete {
		t.Fatalf("execution must complete: %v", res.TargetResults)
	}
	if _, err := os.Lstat(dir); !os.IsNotExist(err) {
		t.Error("the data directory must be removed entirely")
	}
}

func TestExecuteRemovesLinksWithoutFollowing(t *testing.T) {
	plan := makeContentsPlan(t, "keep-target.txt")
	root := plan.DemoTargets[0].Root
	outside := t.TempDir()
	victim := filepath.Join(outside, "victim.txt")
	if err := os.WriteFile(victim, []byte("do not delete"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "redirect")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if err := os.Symlink(victim, filepath.Join(root, "shortcut")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	res := ExecutePlan(plan)
	if !res.Complete {
		t.Fatalf("execution must complete: %v", res.TargetResults)
	}
	if _, err := os.Lstat(victim); err != nil {
		t.Fatalf("the link target must be untouched: %v", err)
	}
	entries, _ := os.ReadDir(root)
	if len(entries) != 0 {
		t.Errorf("the links themselves must be removed, %d remain", len(entries))
	}
}

func TestExecuteRefusesPathsOutsideTheApprovedRoot(t *testing.T) {
	root := t.TempDir()
	secret := filepath.Join(t.TempDir(), "secret.txt")
	if err := os.WriteFile(secret, []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	plan := CleanupPlan{DemoTargets: []PlanTarget{{
		kind:  kindPaths,
		Label: "manipulated",
		Root:  root,
		Paths: []string{secret},
	}}}

	res := ExecutePlan(plan)
	if res.Complete {
		t.Fatal("an escaped path must make the execution incomplete")
	}
	if _, err := os.Lstat(secret); err != nil {
		t.Fatalf("the escaped path must not be deleted: %v", err)
	}
	joined := strings.Join(res.TargetResults[0].Failed, " ")
	if !strings.Contains(joined, "outside the approved cleanup scope") {
		t.Errorf("refusal must be explained, got: %v", res.TargetResults[0].Failed)
	}
}

func TestExecuteFailureBlocksCompletionAndRetries(t *testing.T) {
	plan := makeContentsPlan(t, "one.txt", filepath.Join("sub", "two.txt"))
	root := plan.DemoTargets[0].Root

	// A holder keeps one entry undeletable: a read-only directory on Unix,
	// an open handle without sharing on Windows. This mirrors a running
	// program holding demonstration files open.
	undo := makeUndeletable(t, root)
	t.Cleanup(undo)

	res := ExecutePlan(plan)
	if res.Complete {
		t.Fatal("a failed removal must make the execution incomplete")
	}
	if len(res.FailedTargets()) != 1 {
		t.Fatalf("failures must be recorded, results = %+v", res.TargetResults)
	}
	if !strings.Contains(strings.Join(res.SummaryLines(), "\n"), "reseal was NOT started") {
		t.Error("the report must say the reseal is blocked")
	}

	// After the holder lets go, the same plan executes and completes:
	// removals treat already-missing entries as done, so a retry is safe.
	undo()
	res = ExecutePlan(plan)
	if !res.Complete {
		t.Fatalf("retry must complete: %v", res.TargetResults)
	}
	entries, _ := os.ReadDir(root)
	if len(entries) != 0 {
		t.Errorf("retry must empty the folder, %d entries remain", len(entries))
	}
}

func TestExecuteTreatsVanishedTargetsAsDone(t *testing.T) {
	root := t.TempDir() // never gets any files
	plan := CleanupPlan{DemoTargets: []PlanTarget{{
		kind:  kindDirContents,
		Label: "Downloads",
		Root:  root,
	}}}
	res := ExecutePlan(plan)
	if !res.Complete {
		t.Fatalf("an absent target must be a no-op: %v", res.TargetResults)
	}
}

func TestExecuteMissingExplicitPathsAreNotFailures(t *testing.T) {
	root := t.TempDir()
	plan := CleanupPlan{DemoTargets: []PlanTarget{{
		kind:  kindPaths,
		Label: "Edge",
		Root:  root,
		Paths: []string{filepath.Join(root, "Cache"), filepath.Join(root, "History")},
	}}}
	res := ExecutePlan(plan)
	if !res.Complete || len(res.TargetResults[0].Failed) != 0 {
		t.Fatalf("vanished paths must count as removed: %+v", res.TargetResults)
	}
}
