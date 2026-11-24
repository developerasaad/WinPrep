package handover

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/developerasaad/winprep/internal/artifacts"
	"github.com/developerasaad/winprep/internal/platform"
	"github.com/developerasaad/winprep/internal/platform/fake"
)

const testSID = "S-1-5-21-1-2-3-500"

// setupDemoMachine registers a demo profile in the fake registry and creates
// the given relative paths inside it on disk. It returns the profile root.
func setupDemoMachine(t *testing.T, reg *fake.Registry, relPaths ...string) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "Administrator")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	reg.SetString(platform.HKLM, profileListKey+`\`+testSID, "ProfileImagePath", root)
	reg.SetString(platform.HKU, testSID+`\`+userShellFolders, downloadsValue,
		filepath.Join(root, "Downloads"))
	for _, rel := range relPaths {
		if err := os.MkdirAll(filepath.Join(root, rel), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func writeFile(t *testing.T, path string, size int) {
	t.Helper()
	if err := os.WriteFile(path, make([]byte, size), 0o644); err != nil {
		t.Fatal(err)
	}
}

func demoFacts(names ...string) Facts {
	return Facts{
		User:         platform.UserInfo{SID: testSID, IsElevated: true, IsBuiltinAdmin: true},
		ProcessNames: names,
	}
}

func TestPlanHappyPath(t *testing.T) {
	flow, _, _, store := newTestFlowWithRegistry(t, auditReadyRegistry())
	root := setupDemoMachine(t, flow.Reg.(*fake.Registry),
		"Downloads", "AppData/Local/Temp", "Desktop",
		"AppData/Local/Microsoft/Edge/User Data/Default/Cache",
		"AppData/Local/Microsoft/Edge/User Data/Default/Network",
	)
	writeFile(t, filepath.Join(root, "Downloads", "demo-brochure.pdf"), 2048)
	writeFile(t, filepath.Join(root, "Downloads", "setup.exe"), 1024)
	writeFile(t, filepath.Join(root, "AppData/Local/Temp", "session.tmp"), 512)
	writeFile(t, filepath.Join(root, "AppData/Local/Microsoft/Edge/User Data/Default/Cache", "index"), 4096)
	writeFile(t, filepath.Join(root, "AppData/Local/Microsoft/Edge/User Data/Default/Network/Cookies"), 512)
	writeFile(t, filepath.Join(root, "Desktop", "notes.txt"), 10)

	plan, err := flow.BuildCleanupPlan(demoFacts(), "")
	if err != nil {
		t.Fatalf("BuildCleanupPlan: %v", err)
	}
	if plan.DemoProfile != root {
		t.Errorf("DemoProfile = %q, want %q", plan.DemoProfile, root)
	}

	if len(plan.WinPrepTargets) != 1 {
		t.Fatalf("WinPrepTargets = %+v, want exactly the data directory", plan.WinPrepTargets)
	}
	if plan.WinPrepTargets[0].Root != store.Dir {
		t.Errorf("data dir target = %q, want %q", plan.WinPrepTargets[0].Root, store.Dir)
	}

	find := func(label string) *PlanTarget {
		for i, tg := range plan.DemoTargets {
			if tg.Label == label {
				return &plan.DemoTargets[i]
			}
		}
		return nil
	}
	downloads := find("Downloads")
	if downloads == nil {
		t.Fatal("plan must include the Downloads contents")
	}
	if downloads.Items != 2 || downloads.Size != 2048+1024 {
		t.Errorf("Downloads = %d items, %d bytes; want 2 items, 3072 bytes", downloads.Items, downloads.Size)
	}
	if find("Temporary files") == nil {
		t.Error("plan must include the demo profile's Temp folder")
	}
	edge := find("Microsoft Edge browsing data")
	if edge == nil {
		t.Fatal("plan must include Edge browsing data")
	}
	if len(edge.Paths) != 2 { // Cache directory and Network\Cookies file
		t.Errorf("Edge paths = %v, want Cache and Network\\Cookies", edge.Paths)
	}
	if find("Google Chrome browsing data") != nil {
		t.Error("Chrome was never used; it must not appear on the plan")
	}

	joined := strings.Join(plan.SummaryLines(), "\n")
	for _, want := range []string{"Demonstration profile:", "Downloads", "Microsoft Edge browsing data", "The Desktop folder contains 1 items."} {
		if !strings.Contains(joined, want) {
			t.Errorf("summary missing %q", want)
		}
	}
	if plan.IsBlocked() {
		t.Errorf("unexpected blockers: %v", plan.Blockers)
	}
}

func TestPlanRequiresRID500Session(t *testing.T) {
	flow, _, _, _ := newTestFlowWithRegistry(t, auditReadyRegistry())
	facts := demoFacts()
	facts.User.SID = "S-1-5-21-1-2-3-1013"
	if _, err := flow.BuildCleanupPlan(facts, ""); err == nil {
		t.Fatal("a non-RID-500 session must not produce a cleanup plan")
	}
}

func TestPlanMissingProfileRegistration(t *testing.T) {
	flow, _, _, _ := newTestFlowWithRegistry(t, auditReadyRegistry())
	_, err := flow.BuildCleanupPlan(demoFacts(), "")
	if err == nil || !strings.Contains(err.Error(), "no profile is registered") {
		t.Fatalf("missing ProfileList entry must be an error, got %v", err)
	}
}

func TestPlanRedirectedDownloadsIsNotFollowed(t *testing.T) {
	flow, _, _, _ := newTestFlowWithRegistry(t, auditReadyRegistry())
	setupDemoMachine(t, flow.Reg.(*fake.Registry), "Downloads")
	elsewhere := filepath.Join(t.TempDir(), "elsewhere", "Downloads")
	flow.Reg.(*fake.Registry).SetString(platform.HKU, testSID+`\`+userShellFolders, downloadsValue, elsewhere)

	plan, err := flow.BuildCleanupPlan(demoFacts(), "")
	if err != nil {
		t.Fatal(err)
	}
	for _, tg := range plan.DemoTargets {
		if tg.Label == "Downloads" {
			t.Error("a redirected Downloads must not become a removal target")
		}
	}
	foundNote := false
	for _, n := range plan.Notes {
		if strings.Contains(n, "redirected") {
			foundNote = true
		}
	}
	if !foundNote {
		t.Errorf("the redirection must be reported, notes = %v", plan.Notes)
	}
}

func TestPlanMissingDownloadsValueFallsBack(t *testing.T) {
	flow, _, _, _ := newTestFlowWithRegistry(t, auditReadyRegistry())
	root := setupDemoMachine(t, flow.Reg.(*fake.Registry), "Downloads")
	// Overwrite the GUID value with an empty string, which the planner
	// treats as absent.
	flow.Reg.(*fake.Registry).SetString(platform.HKU, testSID+`\`+userShellFolders, downloadsValue, "")

	plan, err := flow.BuildCleanupPlan(demoFacts(), "")
	if err != nil {
		t.Fatal(err)
	}
	for _, tg := range plan.DemoTargets {
		if tg.Label == "Downloads" && tg.Root == filepath.Join(root, "Downloads") {
			return
		}
	}
	t.Errorf("fallback Downloads target missing, plan = %+v", plan.DemoTargets)
}

func TestPlanTamperedManifestBlocks(t *testing.T) {
	flow, _, _, store := newTestFlowWithRegistry(t, auditReadyRegistry())
	setupDemoMachine(t, flow.Reg.(*fake.Registry))
	m := artifacts.Manifest{}
	m.Add(artifacts.Entry{Type: artifacts.TypeFile, Path: filepath.Join(t.TempDir(), "outside.txt")})
	m.Add(artifacts.Entry{Type: artifacts.TypeRegistry, Path: store.Dir + `\startup`})
	if err := store.Save(m); err != nil {
		t.Fatal(err)
	}

	plan, err := flow.BuildCleanupPlan(demoFacts(), "")
	if err != nil {
		t.Fatal(err)
	}
	if !plan.IsBlocked() {
		t.Fatalf("a tampered manifest must block, blockers = %v", plan.Blockers)
	}
	joined := strings.Join(plan.Blockers, "|")
	if !strings.Contains(joined, "outside WinPrep's data directory") {
		t.Errorf("outside-path entry not reported: %v", plan.Blockers)
	}
	if !strings.Contains(joined, "registry artifact") {
		t.Errorf("registry entry not reported: %v", plan.Blockers)
	}
}

func TestPlanManifestReadFailureBlocks(t *testing.T) {
	flow, _, _, store := newTestFlowWithRegistry(t, auditReadyRegistry())
	setupDemoMachine(t, flow.Reg.(*fake.Registry))
	if err := os.WriteFile(store.Path(), []byte("not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	plan, err := flow.BuildCleanupPlan(demoFacts(), "")
	if err != nil {
		t.Fatal(err)
	}
	if !plan.IsBlocked() {
		t.Fatal("an unreadable manifest must block the cleanup")
	}
}

func TestPlanBrowserRunningBlocks(t *testing.T) {
	flow, _, _, _ := newTestFlowWithRegistry(t, auditReadyRegistry())
	root := setupDemoMachine(t, flow.Reg.(*fake.Registry),
		"AppData/Local/Microsoft/Edge/User Data/Default/Cache")

	plan, err := flow.BuildCleanupPlan(demoFacts("explorer.exe", "msedge.exe"), "")
	if err != nil {
		t.Fatal(err)
	}
	if !plan.IsBlocked() {
		t.Fatal("a running browser must block its own data cleanup")
	}
	if !strings.Contains(strings.Join(plan.Blockers, " "), "Microsoft Edge is still running") {
		t.Errorf("blocker should name Edge: %v", plan.Blockers)
	}
	for _, tg := range plan.DemoTargets {
		if tg.Label == "Microsoft Edge browsing data" {
			t.Error("a running browser's data must not be scheduled for removal")
		}
	}
	_ = root
}

func TestPlanExtraBrowserProfilesAreNoted(t *testing.T) {
	flow, _, _, _ := newTestFlowWithRegistry(t, auditReadyRegistry())
	setupDemoMachine(t, flow.Reg.(*fake.Registry),
		"AppData/Local/Microsoft/Edge/User Data/Default/Cache",
		"AppData/Local/Microsoft/Edge/User Data/Profile 1")

	plan, err := flow.BuildCleanupPlan(demoFacts(), "")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, n := range plan.Notes {
		if strings.Contains(n, "Profile 1") {
			found = true
		}
	}
	if !found {
		t.Errorf("extra browser profiles must be reported, notes = %v", plan.Notes)
	}
}

func TestPlanRunningExeInsideTargetBlocks(t *testing.T) {
	flow, _, _, _ := newTestFlowWithRegistry(t, auditReadyRegistry())
	root := setupDemoMachine(t, flow.Reg.(*fake.Registry), "Downloads")
	self := filepath.Join(root, "Downloads", "winprep.exe")

	plan, err := flow.BuildCleanupPlan(demoFacts(), self)
	if err != nil {
		t.Fatal(err)
	}
	if !plan.IsBlocked() {
		t.Fatal("the running executable inside a cleanup target must block")
	}
	if !strings.Contains(strings.Join(plan.Blockers, " "), "winprep.exe is running from") {
		t.Errorf("blocker should explain the executable location: %v", plan.Blockers)
	}
}

func TestPlanEmptyMachine(t *testing.T) {
	flow, _, _, store := newTestFlowWithRegistry(t, auditReadyRegistry())
	setupDemoMachine(t, flow.Reg.(*fake.Registry))
	if err := os.RemoveAll(store.Dir); err != nil {
		t.Fatal(err)
	}

	plan, err := flow.BuildCleanupPlan(demoFacts(), "")
	if err != nil {
		t.Fatal(err)
	}
	if plan.HasActions() {
		t.Errorf("a bare machine should yield no removals, plan = %+v", plan)
	}
	if !strings.Contains(strings.Join(plan.SummaryLines(), "\n"), "No removable demonstration data was found.") {
		t.Error("an empty plan must say so")
	}
}

func TestContainsPath(t *testing.T) {
	cases := []struct {
		name   string
		parent string
		child  string
		want   bool
	}{
		{"same path", `/Users/Admin/Downloads`, `/Users/Admin/Downloads`, true},
		{"direct child", `/Users/Admin/Downloads`, `/Users/Admin/Downloads/file.txt`, true},
		{"deep child", `/Users/Admin`, `/Users/Admin/AppData/Local/Temp/x.tmp`, true},
		{"sibling", `/Users/Admin/Downloads`, `/Users/Admin/Desktop/file.txt`, false},
		{"prefix not boundary", `/Users/Admin/Down`, `/Users/Admin/Downloads/file.txt`, false},
		{"traversal escapes", `/Users/Admin/Downloads`, `/Users/Admin/Downloads/../../Windows`, false},
		{"different volume", `C:\Users\Admin`, `D:\Users\Admin\file.txt`, false},
		{"case fold", `/users/admin/downloads`, `/Users/Admin/Downloads/file.txt`, true},
		{"empty parent", ``, `/x`, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := containsPath(tc.parent, tc.child); got != tc.want {
				t.Errorf("containsPath(%q, %q) = %v, want %v", tc.parent, tc.child, got, tc.want)
			}
		})
	}
}

func TestProcessRunning(t *testing.T) {
	names := []string{"explorer.exe", "WINWORD.EXE", "msedge.exe"}
	if !processRunning(names, "msedge.exe") {
		t.Error("exact match should be detected")
	}
	if !processRunning(names, "MSEDGE.EXE") {
		t.Error("match should be case-insensitive")
	}
	if processRunning(names, "chrome.exe") {
		t.Error("absent process must not be detected")
	}
}
