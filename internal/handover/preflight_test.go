package handover

import (
	"errors"
	"testing"
	"time"

	"github.com/developerasaad/winprep/internal/audit"
	"github.com/developerasaad/winprep/internal/platform"
)

func baseFacts() Facts {
	return Facts{
		System: platform.SystemInfo{
			OSName:           "Windows 11 Pro",
			Build:            "10.0.22631.4317",
			InstallationType: "Client",
			Architecture:     "x64",
			IsWindows:        true,
		},
		User:           platform.UserInfo{IsElevated: true, SID: "S-1-5-21-1-2-3-500", IsBuiltinAdmin: true},
		Audit:          audit.Assessment{State: audit.StateAuditMode, Summary: "audit mode"},
		SysprepPresent: true,
		Now:            time.Now(),
	}
}

func checkByID(checks []Check, id string) *Check {
	for i := range checks {
		if checks[i].ID == id {
			return &checks[i]
		}
	}
	return nil
}

func TestEvaluateVerdict(t *testing.T) {
	tests := []struct {
		name   string
		checks []Check
		want   Verdict
	}{
		{"all pass", []Check{{Severity: Pass}, {Severity: Pass}}, VerdictReady},
		{"attention only", []Check{{Severity: Pass}, {Severity: Attention}}, VerdictAttention},
		{"blocker wins", []Check{{Severity: Attention}, {Severity: Blocker}}, VerdictBlocked},
		{"unknown blocks", []Check{{Severity: Pass}, {Severity: UnknownSeverity}}, VerdictBlocked},
		{"empty is ready", nil, VerdictReady},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := EvaluateVerdict(tt.checks); got != tt.want {
				t.Errorf("EvaluateVerdict() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestBuildChecksPrivileges(t *testing.T) {
	f := baseFacts()
	f.User.IsElevated = false
	c := checkByID(BuildChecks(f), "privileges")
	if c.Severity != Blocker {
		t.Errorf("non-elevated must block, got %v", c.Severity)
	}
	if c.NextStep == "" {
		t.Error("blocker must explain the next step")
	}

	f.User.IsElevated = true
	c = checkByID(BuildChecks(f), "privileges")
	if c.Severity != Pass {
		t.Errorf("elevated should pass, got %v", c.Severity)
	}
}

func TestBuildChecksWindowsSupport(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Facts)
		want   Severity
	}{
		{"windows 11 client x64", func(f *Facts) {}, Pass},
		{"not windows", func(f *Facts) { f.System.IsWindows = false }, Blocker},
		{"server", func(f *Facts) { f.System.InstallationType = "Server" }, Blocker},
		{"arm64", func(f *Facts) { f.System.Architecture = "ARM64" }, Blocker},
		{"build unknown", func(f *Facts) { f.System.Build = "" }, UnknownSeverity},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := baseFacts()
			tt.mutate(&f)
			if c := checkByID(BuildChecks(f), "windows"); c.Severity != tt.want {
				t.Errorf("severity = %v, want %v", c.Severity, tt.want)
			}
		})
	}
}

func TestBuildChecksAuditStates(t *testing.T) {
	tests := []struct {
		state audit.State
		want  Severity
	}{
		{audit.StateAuditMode, Pass},
		{audit.StateNormalInstalled, Blocker},
		{audit.StateOOBE, Blocker},
		{audit.StateUncertain, UnknownSeverity},
		{audit.StateUnknown, UnknownSeverity},
	}
	for _, tt := range tests {
		f := baseFacts()
		f.Audit = audit.Assessment{State: tt.state, Summary: "summary"}
		c := checkByID(BuildChecks(f), "audit")
		if c.Severity != tt.want {
			t.Errorf("state %v: severity = %v, want %v", tt.state, c.Severity, tt.want)
		}
		if tt.want != Pass && c.NextStep == "" {
			t.Errorf("state %v: non-pass check must have a next step", tt.state)
		}
	}
}

func TestBuildChecksSysprep(t *testing.T) {
	f := baseFacts()
	if c := checkByID(BuildChecks(f), "sysprep"); c.Severity != Pass {
		t.Errorf("present sysprep should pass, got %v", c.Severity)
	}
	f.SysprepPresent = false
	if c := checkByID(BuildChecks(f), "sysprep"); c.Severity != Blocker {
		t.Errorf("missing sysprep must block, got %v", c.Severity)
	}
	f.SysprepErr = errors.New("access denied")
	if c := checkByID(BuildChecks(f), "sysprep"); c.Severity != UnknownSeverity {
		t.Errorf("unreadable sysprep state must be unknown, got %v", c.Severity)
	}
}

func TestBuildChecksSysprepConflict(t *testing.T) {
	// No Sysprep process: pass.
	f := baseFacts()
	if c := checkByID(BuildChecks(f), "sysprep-idle"); c.Severity != Pass {
		t.Errorf("no sysprep process should pass, got %v", c.Severity)
	}

	// A sysprep.exe process with freshly written logs is an active operation.
	f.SysprepRunning = true
	f.SysprepLogTimes = []time.Time{f.Now.Add(-20 * time.Second)}
	if c := checkByID(BuildChecks(f), "sysprep-idle"); c.Severity != Blocker {
		t.Errorf("an active Sysprep operation must block, got %v", c.Severity)
	}

	// A sysprep.exe process whose logs are stale is the idle window Audit
	// Mode opens at sign-in: attention, not a blocker.
	f.SysprepLogTimes = []time.Time{f.Now.Add(-48 * time.Hour)}
	idle := checkByID(BuildChecks(f), "sysprep-idle")
	if idle.Severity != Attention {
		t.Errorf("an idle Sysprep window should be attention, got %v", idle.Severity)
	}
	if !contains(idle.Detail, "idle") {
		t.Errorf("the idle case should say so, detail = %q", idle.Detail)
	}

	// A sysprep.exe process with no logs at all is also treated as idle:
	// a machine that never ran Sysprep has no Panther logs.
	f.SysprepLogTimes = nil
	if c := checkByID(BuildChecks(f), "sysprep-idle"); c.Severity != Attention {
		t.Errorf("a process with no logs should be attention, got %v", c.Severity)
	}

	// The process exists but its activity cannot be read: unknown, which
	// blocks like a blocker.
	f.SysprepLogErr = errors.New("access denied")
	if c := checkByID(BuildChecks(f), "sysprep-idle"); c.Severity != UnknownSeverity {
		t.Errorf("unreadable log state must be unknown, got %v", c.Severity)
	}

	// A process listing failure remains unknown.
	f = baseFacts()
	f.ProcsErr = errors.New("snapshot failed")
	if c := checkByID(BuildChecks(f), "sysprep-idle"); c.Severity != UnknownSeverity {
		t.Errorf("process list failure must be unknown, got %v", c.Severity)
	}
}

func TestBuildChecksPendingReboot(t *testing.T) {
	tests := []struct {
		name    string
		cbs, wu bool
		fileRen bool
		want    Severity
	}{
		{"nothing pending", false, false, false, Pass},
		{"cbs pending blocks", true, false, false, Blocker},
		{"wu pending blocks", false, true, false, Blocker},
		{"both block", true, true, false, Blocker},
		{"file renames are attention", false, false, true, Attention},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := baseFacts()
			f.PendingCBSReboot, f.PendingWUReboot, f.PendingFileRename = tt.cbs, tt.wu, tt.fileRen
			if c := checkByID(BuildChecks(f), "pending-reboot"); c.Severity != tt.want {
				t.Errorf("severity = %v, want %v", c.Severity, tt.want)
			}
		})
	}
}

func TestBuildChecksAccountsAndProfiles(t *testing.T) {
	// Extra accounts survive the reseal and would be handed to the buyer,
	// so they block rather than merely warn.
	f := baseFacts()
	f.Accounts = []AccountFact{{Name: "testuser", SID: "S-1-5-21-1-2-3-1001"}}
	f.Profiles = []ProfileFact{{SID: "S-1-5-21-1-2-3-1001", Path: `C:\Users\testuser`}}
	checks := BuildChecks(f)
	if c := checkByID(checks, "accounts"); c.Severity != Blocker {
		t.Errorf("extra accounts must block, got %v", c.Severity)
	}
	if c := checkByID(checks, "profiles"); c.Severity != Blocker {
		t.Errorf("extra profiles must block, got %v", c.Severity)
	}
	if EvaluateVerdict(checks) != VerdictBlocked {
		t.Errorf("extra accounts and profiles must yield a blocked verdict, got %v", EvaluateVerdict(checks))
	}

	// Orphaned profile folders block as well.
	f.Accounts = nil
	f.Profiles = nil
	f.OrphanDirs = []string{`C:\Users\leftover`}
	if c := checkByID(BuildChecks(f), "profiles"); c.Severity != Blocker {
		t.Errorf("orphaned profile folders must block, got %v", c.Severity)
	}

	f.OrphanDirs = nil
	f.AccountsErr = errors.New("sam exploded")
	if c := checkByID(BuildChecks(f), "accounts"); c.Severity != UnknownSeverity {
		t.Errorf("account query failure must be unknown, got %v", c.Severity)
	}
}

func TestReadyPathYieldsReadyVerdict(t *testing.T) {
	checks := BuildChecks(baseFacts())
	if got := EvaluateVerdict(checks); got != VerdictReady {
		t.Fatalf("clean base facts should be ready, got %v", got)
	}
}

func TestSIDRID(t *testing.T) {
	tests := map[string]int{
		"S-1-5-21-1-2-3-500":  500,
		"S-1-5-21-1-2-3-1001": 1001,
		"S-1-5-18":            18,
		"":                    0,
		"garbage":             0,
		"S-1-5-21-1-2-3-":     0,
		"S-1-5-21-1-2-3-12x":  0,
	}
	for sid, want := range tests {
		if got := sidRID(sid); got != want {
			t.Errorf("sidRID(%q) = %d, want %d", sid, got, want)
		}
	}
}

func TestExplainListsAllChecks(t *testing.T) {
	r := PreflightResult{Verdict: VerdictAttention, Checks: BuildChecks(baseFacts())}
	lines := r.Explain()
	joined := ""
	for _, l := range lines {
		joined += l + "\n"
	}
	for _, want := range []string{"Overall: Requires attention", "Audit Mode: Passed", "Administrative privileges: Passed"} {
		if !contains(joined, want) {
			t.Errorf("Explain() output missing %q", want)
		}
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (func() bool {
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	})()
}
