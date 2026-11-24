package handover

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/developerasaad/winprep/internal/artifacts"
	"github.com/developerasaad/winprep/internal/audit"
	"github.com/developerasaad/winprep/internal/platform"
)

// Well-known registry locations used during inspection.
const (
	profileListKey = `SOFTWARE\Microsoft\Windows NT\CurrentVersion\ProfileList`
	cbsRebootKey   = `SOFTWARE\Microsoft\Windows\CurrentVersion\Component Based Servicing\RebootPending`
	wuRebootKey    = `SOFTWARE\Microsoft\Windows\CurrentVersion\WindowsUpdate\Auto Update\RebootRequired`
	sessionMgrKey  = `SYSTEM\CurrentControlSet\Control\Session Manager`
)

// Flow wires the platform services into the handover steps. Both the CLI and
// the TUI drive the same Flow; there is exactly one implementation of the
// handover procedure.
type Flow struct {
	Sys     platform.System
	Reg     platform.Registry
	Procs   platform.ProcessLister
	Audit   *audit.Service
	Store   *artifacts.Store
	Sysprep SysprepRunner

	// UsersDir is the profiles root used to find orphan profile folders.
	UsersDir string
}

// Inspect gathers fresh facts and evaluates them. Facts are always gathered
// live: no cached state may authorize a handover.
func (f *Flow) Inspect(ctx context.Context) (PreflightResult, error) {
	facts, err := f.Gather(ctx)
	if err != nil {
		return PreflightResult{}, err
	}
	checks := BuildChecks(facts)
	return PreflightResult{Verdict: EvaluateVerdict(checks), Checks: checks}, nil
}

// Gather collects the raw facts. A failure to read a fundamental signal (the
// audit state or system facts) aborts the whole inspection; individual
// subsystem failures are recorded as facts errors and surface as Unknown
// checks.
func (f *Flow) Gather(ctx context.Context) (Facts, error) {
	var facts Facts

	sys, err := f.Sys.SystemInfo(ctx)
	if err != nil {
		return facts, fmt.Errorf("reading system information: %w", err)
	}
	facts.System = sys

	user, err := f.Sys.UserInfo(ctx)
	if err != nil {
		return facts, fmt.Errorf("reading user information: %w", err)
	}
	facts.User = user

	assessment, err := f.Audit.Inspect(ctx)
	if err != nil {
		return facts, fmt.Errorf("determining setup state: %w", err)
	}
	facts.Audit = assessment

	facts.Accounts, facts.AccountsErr = f.listAccounts(ctx)
	facts.Profiles, facts.ProfilesErr, facts.OrphanDirs = f.listProfiles()
	facts.SysprepPresent, facts.SysprepErr = f.checkSysprep()
	facts.SysprepRunning, facts.ProcsErr = f.checkSysprepRunning(ctx)
	facts.PendingCBSReboot, facts.PendingWUReboot, facts.PendingFileRename, facts.RebootErr = f.readRebootState()
	if m, err := f.Store.Load(); err != nil {
		facts.ArtifactsErr = err
	} else {
		facts.Artifacts = m.Entries
	}
	return facts, nil
}

func (f *Flow) listAccounts(ctx context.Context) ([]AccountFact, error) {
	accounts, err := f.Sys.LocalAccounts(ctx)
	if err != nil {
		return nil, fmt.Errorf("querying local accounts: %w", err)
	}
	// Only accounts with a relative identifier of 1000 or higher are
	// "extra"; the built-in accounts (500 Administrator, 501 Guest, and
	// the 501-599 well-known range) are expected on every installation.
	var extra []AccountFact
	for _, a := range accounts {
		if a.RID >= 1000 {
			extra = append(extra, AccountFact{Name: a.Name, SID: a.SID, Disabled: a.Disabled})
		}
	}
	return extra, nil
}

func (f *Flow) listProfiles() (extra []ProfileFact, profilesErr error, orphans []string) {
	subKeys, err := f.Reg.SubKeyNames(platform.HKLM, profileListKey)
	if err != nil {
		return nil, err, nil
	}
	var registered []ProfileFact
	for _, sid := range subKeys {
		path, ok, err := f.Reg.StringValue(platform.HKLM, profileListKey+`\`+sid, "ProfileImagePath")
		if err != nil {
			return nil, err, nil
		}
		if !ok {
			continue
		}
		if sidRID(sid) >= 1000 {
			extra = append(extra, ProfileFact{SID: sid, Path: path})
		}
		registered = append(registered, ProfileFact{SID: sid, Path: path})
	}

	// Folders under the profiles root that no registered profile claims and
	// that are not part of the standard set are worth a look before a sale.
	if f.UsersDir != "" {
		entries, err := filepath.Glob(filepath.Join(f.UsersDir, "*"))
		if err == nil {
			for _, dir := range entries {
				name := filepath.Base(dir)
				if isStandardProfileDir(name) {
					continue
				}
				claimed := false
				for _, p := range registered {
					if strings.EqualFold(filepath.Base(p.Path), name) {
						claimed = true
						break
					}
				}
				if !claimed {
					orphans = append(orphans, dir)
				}
			}
		}
	}
	return extra, nil, orphans
}

// isStandardProfileDir names the folders every Windows installation carries.
// "All Users" and "Default User" are junctions that appear in directory
// listings but are not real profiles.
func isStandardProfileDir(name string) bool {
	switch strings.ToLower(name) {
	case "public", "default", "administrator", "all users", "default user", "defaultapppool", "wdagutilityaccount":
		return true
	}
	return false
}

func (f *Flow) checkSysprep() (bool, error) {
	if _, err := f.Sysprep.SysprepPath(); err != nil {
		return false, nil
	}
	return true, nil
}

func (f *Flow) checkSysprepRunning(ctx context.Context) (bool, error) {
	names, err := f.Procs.ProcessNames(ctx)
	if err != nil {
		return false, err
	}
	for _, n := range names {
		if strings.EqualFold(n, "sysprep.exe") {
			return true, nil
		}
	}
	return false, nil
}

func (f *Flow) readRebootState() (cbs, wu, fileRename bool, err error) {
	cbs, err = f.Reg.KeyExists(platform.HKLM, cbsRebootKey)
	if err != nil {
		return false, false, false, fmt.Errorf("reading component servicing state: %w", err)
	}
	wu, err = f.Reg.KeyExists(platform.HKLM, wuRebootKey)
	if err != nil {
		return false, false, false, fmt.Errorf("reading update state: %w", err)
	}
	ops, _, err := f.Reg.MultiStringValue(platform.HKLM, sessionMgrKey, "PendingFileRenameOperations")
	if err != nil {
		return false, false, false, fmt.Errorf("reading pending file operations: %w", err)
	}
	fileRename = len(ops) > 0
	return cbs, wu, fileRename, nil
}

// sidRID extracts the relative identifier (the final component) of an SID
// such as S-1-5-21-1000-2000-3000-1013. Unparseable SIDs return 0, which is
// never >= 1000 and therefore never flags an account as extra.
func sidRID(sid string) int {
	i := strings.LastIndex(sid, "-")
	if i < 0 || i == len(sid)-1 {
		return 0
	}
	rid := 0
	for _, c := range sid[i+1:] {
		if c < '0' || c > '9' {
			return 0
		}
		rid = rid*10 + int(c-'0')
	}
	return rid
}
