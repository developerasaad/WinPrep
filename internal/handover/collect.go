package handover

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

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

// Controller owns the Clean & Finish workflow: preflight, cleanup-plan
// construction, approval state, final validation, cleanup execution, reseal
// invocation, and outcome reporting. Both the CLI and the TUI present its
// results and collect the confirmation; neither decides for itself when it
// is safe to delete files or invoke Sysprep.
type Controller struct {
	Sys     platform.System
	Reg     platform.Registry
	Procs   platform.ProcessLister
	Audit   *audit.Service
	Store   *artifacts.Store
	Sysprep SysprepRunner

	// SysprepReports distinguishes an idle System Preparation Tool window
	// from an active Sysprep operation.
	SysprepReports platform.SysprepMonitor
	// Lock prevents concurrent Clean & Finish operations across
	// processes. A nil Lock disables the check (unit tests).
	Lock platform.InstanceLock

	// UsersDir is the profiles root used to find orphan profile folders.
	UsersDir string
}

// Inspect gathers fresh facts and evaluates them. Facts are always gathered
// live: no cached inspection result may authorize a handover.
func (c *Controller) Inspect(ctx context.Context) (PreflightResult, error) {
	facts, err := c.Gather(ctx)
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
func (c *Controller) Gather(ctx context.Context) (Facts, error) {
	var facts Facts

	sys, err := c.Sys.SystemInfo(ctx)
	if err != nil {
		return facts, fmt.Errorf("reading system information: %w", err)
	}
	facts.System = sys

	user, err := c.Sys.UserInfo(ctx)
	if err != nil {
		return facts, fmt.Errorf("reading user information: %w", err)
	}
	facts.User = user

	assessment, err := c.Audit.Inspect(ctx)
	if err != nil {
		return facts, fmt.Errorf("determining setup state: %w", err)
	}
	facts.Audit = assessment

	facts.Accounts, facts.AccountsErr = c.listAccounts(ctx)
	facts.Profiles, facts.ProfilesErr, facts.OrphanDirs = c.listProfiles()
	facts.SysprepPresent, facts.SysprepErr = c.checkSysprep()
	facts.ProcessNames, facts.ProcsErr = c.Procs.ProcessNames(ctx)
	facts.SysprepRunning = processRunning(facts.ProcessNames, "sysprep.exe")
	facts.SysprepLogTimes, facts.SysprepLogErr = c.sysprepLogTimes(ctx)
	facts.PendingCBSReboot, facts.PendingWUReboot, facts.PendingFileRename, facts.RebootErr = c.readRebootState()
	facts.Now = time.Now()
	return facts, nil
}

func (c *Controller) sysprepLogTimes(ctx context.Context) ([]time.Time, error) {
	if c.SysprepReports == nil {
		return nil, nil
	}
	return c.SysprepReports.SysprepLogTimes(ctx)
}

func (c *Controller) listAccounts(ctx context.Context) ([]AccountFact, error) {
	accounts, err := c.Sys.LocalAccounts(ctx)
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

func (c *Controller) listProfiles() (extra []ProfileFact, profilesErr error, orphans []string) {
	subKeys, err := c.Reg.SubKeyNames(platform.HKLM, profileListKey)
	if err != nil {
		return nil, err, nil
	}
	var registered []ProfileFact
	for _, sid := range subKeys {
		path, ok, err := c.Reg.StringValue(platform.HKLM, profileListKey+`\`+sid, "ProfileImagePath")
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
	if c.UsersDir != "" {
		entries, err := filepath.Glob(filepath.Join(c.UsersDir, "*"))
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

func (c *Controller) checkSysprep() (bool, error) {
	if _, err := c.Sysprep.SysprepPath(); err != nil {
		return false, nil
	}
	return true, nil
}

func (c *Controller) readRebootState() (cbs, wu, fileRename bool, err error) {
	cbs, err = c.Reg.KeyExists(platform.HKLM, cbsRebootKey)
	if err != nil {
		return false, false, false, fmt.Errorf("reading component servicing state: %w", err)
	}
	wu, err = c.Reg.KeyExists(platform.HKLM, wuRebootKey)
	if err != nil {
		return false, false, false, fmt.Errorf("reading update state: %w", err)
	}
	ops, _, err := c.Reg.MultiStringValue(platform.HKLM, sessionMgrKey, "PendingFileRenameOperations")
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
	for _, ch := range sid[i+1:] {
		if ch < '0' || ch > '9' {
			return 0
		}
		rid = rid*10 + int(ch-'0')
	}
	return rid
}
