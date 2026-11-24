package handover

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/developerasaad/winprep/internal/artifacts"
	"github.com/developerasaad/winprep/internal/platform"
)

// The demonstration environment is the profile of the built-in Administrator
// account, because Audit Mode signs every session in as that account. The
// shell folder GUIDs below are the stable, language-neutral registry names for
// the folders WinPrep inspects; physical folder names vary with redirection,
// so nothing is assumed to be English.
const (
	userShellFolders = `SOFTWARE\Microsoft\Windows\CurrentVersion\Explorer\User Shell Folders`
	downloadsValue   = "{374DE290-123F-4565-9164-39C4925E467B}"
)

// browserDef describes a supported browser: where its per-user data lives
// (relative to the profile root) and the process that locks it while running.
type browserDef struct {
	name    string
	dataDir string
	process string
}

var browsers = []browserDef{
	{name: "Microsoft Edge", dataDir: `AppData\Local\Microsoft\Edge\User Data`, process: "msedge.exe"},
	{name: "Google Chrome", dataDir: `AppData\Local\Google\Chrome\User Data`, process: "chrome.exe"},
}

// browserDataPaths are the cache and personal-data locations inside a
// browser's "Default" profile directory. Caches are safe to delete and
// regenerate; history, cookies, and saved logins are exactly the traces a
// buyer should not inherit from the demonstration.
var browserDataPaths = []string{
	"Cache",
	"Code Cache",
	"GPUCache",
	"History",
	"Cookies",
	`Network\Cookies`,
	"Login Data",
	"Web Data",
}

// reviewFolders are shell folders whose contents WinPrep reports but never
// deletes. Counts are top-level entries only.
var reviewFolders = []string{"Desktop", "Documents", "Pictures", "Videos", "Music"}

// BuildCleanupPlan constructs the read-only cleanup proposal from freshly
// gathered facts. It performs no modification of any kind. selfPath is the
// path of the running executable (empty when unknown); the planner refuses to
// approve a plan whose cleanup would delete the running program itselc.
func (c *Controller) BuildCleanupPlan(facts Facts, selfPath string) (CleanupPlan, error) {
	plan := CleanupPlan{}

	// The demonstration profile must be resolvable through the session user's
	// SID. Only the RID-500 account's profile can be the demo environment,
	// and only a registry-resolved path is trusted for it.
	if sidRID(facts.User.SID) != 500 {
		return plan, fmt.Errorf("cannot establish the demonstration profile: the session user %q is not the built-in Administrator",
			facts.User.SID)
	}
	root, err := c.profileRoot(facts.User.SID)
	if err != nil {
		return plan, fmt.Errorf("cannot establish the demonstration profile: %w", err)
	}
	plan.DemoProfile = root

	// WinPrep's own files: the data directory is owned by WinPrep by
	// construction. Manifest entries must agree with that ownership rule; a
	// manifest that claims paths outside the data directory is treated as
	// tampered, not executed.
	if _, statErr := os.Lstat(c.Store.Dir); statErr == nil {
		plan.WinPrepTargets = append(plan.WinPrepTargets, PlanTarget{
			kind:  kindDataDir,
			Label: "WinPrep data directory",
			Root:  c.Store.Dir,
		})
	}
	manifest, mErr := c.Store.Load()
	switch {
	case mErr != nil:
		plan.Blockers = append(plan.Blockers, "the WinPrep artifact manifest could not be read: "+mErr.Error())
	default:
		for _, entry := range manifest.Entries {
			if err := c.Store.EnsureInsideDataDir(entry.Path); err != nil {
				plan.Blockers = append(plan.Blockers, "the artifact manifest records a path outside WinPrep's data directory ("+
					entry.Path+"); it will not be removed. Check "+c.Store.Path()+" and remove the entry manually.")
			}
			if entry.Type == artifacts.TypeRegistry {
				plan.Blockers = append(plan.Blockers, "the artifact manifest records a registry artifact ("+entry.Path+
					") that this WinPrep version cannot undo; it will not be touched.")
			}
		}
	}

	// Demonstration data inside the demo profile.
	if err := c.planDownloads(&plan, root, facts.User.SID); err != nil {
		return plan, err
	}
	c.planTemp(&plan, root)
	for _, b := range browsers {
		c.planBrowser(&plan, root, b, facts)
	}
	c.planReviewFolders(&plan, root)

	// The running executable must not sit inside a folder the cleanup empties;
	// Windows locks running executables, so deletion would fail midway.
	if selfPath != "" && plan.HasActions() {
		for _, group := range [][]PlanTarget{plan.WinPrepTargets, plan.DemoTargets} {
			for _, t := range group {
				if containsPath(t.Root, selfPath) {
					plan.Blockers = append(plan.Blockers, "winprep.exe is running from "+selfPath+
						", which the cleanup will empty. Move it to a USB stick or another folder outside the demonstration data first.")
				}
			}
		}
	}
	return plan, nil
}

// profileRoot resolves the profile directory of the given SID through
// ProfileList. This is the authoritative source for profile locations, and
// the resolution happens by SID, never by account name.
func (c *Controller) profileRoot(sid string) (string, error) {
	path, ok, err := c.Reg.StringValue(platform.HKLM, profileListKey+`\`+sid, "ProfileImagePath")
	if err != nil {
		return "", err
	}
	if !ok || strings.TrimSpace(path) == "" {
		return "", fmt.Errorf("no profile is registered for %s", sid)
	}
	return path, nil
}

// planDownloads adds the Downloads folder of the demo profile. The folder's
// location is read from the per-user shell-folders registry value so a
// redirected or differently named folder is still found; if the redirect
// points outside the profile, the data is reported for manual review instead
// of being chased outside the approved scope.
func (c *Controller) planDownloads(plan *CleanupPlan, root, sid string) error {
	value, ok, err := c.Reg.StringValue(platform.HKU,
		sid+"\\"+userShellFolders, downloadsValue)
	if err != nil {
		return fmt.Errorf("reading the Downloads location: %w", err)
	}
	downloads := filepath.Join(root, "Downloads")
	if ok && strings.TrimSpace(value) != "" {
		if filepath.IsAbs(value) {
			downloads = filepath.Clean(value)
		} else {
			downloads = filepath.Join(root, value)
		}
	}
	if !containsPath(root, downloads) {
		plan.Notes = append(plan.Notes, "Downloads is redirected to "+downloads+
			", outside the demonstration profile. Clean it yourself; WinPrep does not follow it.")
		return nil
	}
	addDirContentsTarget(plan, root, "Downloads", downloads)
	return nil
}

// planTemp adds the demo profile's local Temp folder.
func (c *Controller) planTemp(plan *CleanupPlan, root string) {
	addDirContentsTarget(plan, root, "Temporary files", filepath.Join(root, "AppData", "Local", "Temp"))
}

// planBrowser adds the browsing data of one browser when its profile exists.
// A running browser locks its files, so the data becomes a blocker rather
// than a doomed removal. Extra profiles beyond Default are reported because
// they are outside the covered scope.
func (c *Controller) planBrowser(plan *CleanupPlan, root string, b browserDef, facts Facts) {
	userData := joinRel(root, b.dataDir)
	profileDir := filepath.Join(userData, "Default")
	if _, err := os.Lstat(profileDir); err != nil {
		return // the browser was never used on this machine
	}

	if processRunning(facts.ProcessNames, b.process) {
		plan.Blockers = append(plan.Blockers, b.name+" is still running, so its browsing data cannot be removed. "+
			"Close it fully (Startup Boost can keep it in the background) and run the cleanup again.")
		return
	}

	var paths []string
	var items int
	var size uint64
	for _, rel := range browserDataPaths {
		p := joinRel(profileDir, rel)
		if _, err := os.Lstat(p); err != nil {
			continue
		}
		paths = append(paths, p)
		n, s := countPath(p)
		items += n
		size += s
	}
	if len(paths) > 0 {
		plan.DemoTargets = append(plan.DemoTargets, PlanTarget{
			kind:  kindPaths,
			Label: b.name + " browsing data",
			Root:  profileDir,
			Paths: paths,
			Items: items,
			Size:  size,
		})
	}

	if entries, err := os.ReadDir(userData); err == nil {
		var extra []string
		for _, e := range entries {
			name := e.Name()
			if e.IsDir() && strings.HasPrefix(name, "Profile ") && name != "Default" {
				extra = append(extra, name)
			}
		}
		if len(extra) > 0 {
			plan.Notes = append(plan.Notes, b.name+" has additional profiles ("+strings.Join(extra, ", ")+
				") that the cleanup does not cover. Review them yourselc.")
		}
	}
}

// planReviewFolders counts the shell folders WinPrep deliberately does not
// touch, so the operator knows what still needs a manual look.
func (c *Controller) planReviewFolders(plan *CleanupPlan, root string) {
	for _, name := range reviewFolders {
		dir := filepath.Join(root, name)
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		if len(entries) > 0 {
			plan.Notes = append(plan.Notes, fmt.Sprintf("The %s folder contains %d items.", name, len(entries)))
		}
	}
}

// addDirContentsTarget appends a directory-contents target when the directory
// exists, counting what a removal would currently delete.
func addDirContentsTarget(plan *CleanupPlan, root string, label, dir string) {
	info, err := os.Lstat(dir)
	if err != nil || !info.IsDir() {
		return
	}
	items, size := countDirContents(dir)
	plan.DemoTargets = append(plan.DemoTargets, PlanTarget{
		kind:  kindDirContents,
		Label: label,
		Root:  dir,
		Items: items,
		Size:  size,
	})
}

// joinRel joins a Windows-style relative path (backslash-separated) onto a
// base directory for the current OS, so the constants can be written the way
// they appear in Windows documentation while still working on the development
// and CI platforms.
func joinRel(base, rel string) string {
	return filepath.Join(base, filepath.FromSlash(strings.ReplaceAll(rel, `\`, "/")))
}

// processRunning reports whether a process with the given name is running.
func processRunning(names []string, process string) bool {
	for _, n := range names {
		if strings.EqualFold(n, process) {
			return true
		}
	}
	return false
}

// countDirContents counts the entries below dir (recursively) without
// following symlinks or junctions. Only regular files contribute size.
func countDirContents(dir string) (items int, size uint64) {
	_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // unreadable entries are counted as absent
		}
		if path == dir {
			return nil
		}
		items++
		if d.Type().IsRegular() {
			if info, err := d.Info(); err == nil {
				size += uint64(info.Size())
			}
		}
		return nil
	})
	return items, size
}

// countPath counts one explicit path: a file counts as one item, a directory
// as its recursive contents, a link as one item with no size.
func countPath(path string) (int, uint64) {
	info, err := os.Lstat(path)
	if err != nil {
		return 0, 0
	}
	if info.IsDir() && info.Mode()&os.ModeSymlink == 0 {
		return countDirContents(path)
	}
	if info.Mode().IsRegular() {
		return 1, uint64(info.Size())
	}
	return 1, 0
}

// containsPath reports whether child is equal to or inside parent. Comparison
// is lexical on cleaned, slash-normalized, case-folded paths: Windows paths
// are case-insensitive. This is the outer containment guard; the executor
// additionally refuses to traverse reparse points, so a link swapped in after
// planning cannot redirect a deletion outside the approved scope.
func containsPath(parent, child string) bool {
	p := normalizePath(parent)
	c := normalizePath(child)
	if p == "" || c == "" {
		return false
	}
	if c == p {
		return true
	}
	return strings.HasPrefix(c, p+"/")
}

func normalizePath(path string) string {
	if path == "" {
		return ""
	}
	return strings.ToLower(strings.TrimSuffix(filepath.ToSlash(filepath.Clean(path)), "/"))
}
