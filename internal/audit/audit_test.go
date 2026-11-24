package audit

import (
	"errors"
	"testing"

	"github.com/developerasaad/winprep/internal/platform"
	"github.com/developerasaad/winprep/internal/platform/fake"
)

// realAuditMachine builds the signals a genuine Audit Mode session exposes
// in the field: ImageState set (never StateName), the audit boot flag set,
// no "audit.exe" anywhere in CmdLine, and the built-in Administrator signed
// in. Windows 10-era machines in Audit Mode report IMAGE_STATE_UNDEPLOYABLE;
// older OEM images were observed with the RESEAL_TO_AUDIT value.
func realAuditMachine(imageState string) platform.AuditSignals {
	return platform.AuditSignals{
		SetupImageState:           imageState,
		HaveSetupImageState:       true,
		SetupCmdLine:              "",
		HaveSetupCmdLine:          true,
		SystemSetupInProgress:     0,
		HaveSystemSetupInProgress: true,
		OOBEInProgress:            0,
		HaveOOBEInProgress:        true,
		AuditBoot:                 1,
		HaveAuditBoot:             true,
		CurrentUserSID:            "S-1-5-21-1000-2000-3000-500",
		CurrentUserIsBuiltinAdmin: true,
	}
}

func TestEvaluate(t *testing.T) {
	tests := []struct {
		name string
		sig  platform.AuditSignals
		want State
	}{
		// Confirmed Audit Mode.
		{
			name: "audit mode, Win10-era field values (UNDEPLOYABLE, AuditBoot, admin)",
			sig:  realAuditMachine(stateUndeployable),
			want: StateAuditMode,
		},
		{
			name: "audit mode, OEM-era field values (SPECIALIZE_RESEAL_TO_AUDIT)",
			sig:  realAuditMachine(stateSpecializeResealAudit),
			want: StateAuditMode,
		},
		{
			name: "audit mode, GENERALIZE_RESEAL_TO_AUDIT",
			sig:  realAuditMachine(stateGeneralizeResealAudit),
			want: StateAuditMode,
		},
		{
			name: "audit mode with AuditBootVolatile instead of AuditBoot (Win11 observation)",
			sig: func() platform.AuditSignals {
				s := realAuditMachine(stateUndeployable)
				s.HaveAuditBoot = false
				s.AuditBoot = 0
				s.AuditBootVolatile, s.HaveAuditBootVolatile = 1, true
				return s
			}(),
			want: StateAuditMode,
		},
		{
			// CmdLine is an observation only. A machine that carries pending
			// setup work (SetupComplete.cmd) or an OOBE boot helper in that
			// slot must not lose its Audit Mode verdict over it, and a
			// machine that never wrote "audit.exe" there must still be
			// recognized.
			name: "audit mode with unexpected CmdLine values stays detected",
			sig: func() platform.AuditSignals {
				s := realAuditMachine(stateUndeployable)
				s.SetupCmdLine = `C:\Windows\Setup\Scripts\SetupComplete.cmd`
				return s
			}(),
			want: StateAuditMode,
		},
		{
			name: "audit mode with oobe windeploy CmdLine stays detected",
			sig: func() platform.AuditSignals {
				s := realAuditMachine(stateUndeployable)
				s.SetupCmdLine = `oobe\windeploy.exe`
				return s
			}(),
			want: StateAuditMode,
		},
		{
			name: "audit mode with CmdLine value entirely absent",
			sig: func() platform.AuditSignals {
				s := realAuditMachine(stateUndeployable)
				s.HaveSetupCmdLine = false
				return s
			}(),
			want: StateAuditMode,
		},
		{
			name: "audit mode corroborated by matching State.ini",
			sig: func() platform.AuditSignals {
				s := realAuditMachine(stateUndeployable)
				s.StateIniImageState, s.HaveStateIniImageState = stateUndeployable, true
				return s
			}(),
			want: StateAuditMode,
		},

		// Ordinary installed Windows.
		{
			name: "normal installed",
			sig: func() platform.AuditSignals {
				s := realAuditMachine(stateComplete)
				s.HaveAuditBoot = false
				s.AuditBoot = 0
				return s
			}(),
			want: StateNormalInstalled,
		},
		{
			name: "normal installed with empty CmdLine value present",
			sig: func() platform.AuditSignals {
				s := realAuditMachine(stateComplete)
				s.HaveAuditBoot = false
				s.AuditBoot = 0
				return s
			}(),
			want: StateNormalInstalled,
		},
		{
			name: "normal installed while signed in as enabled built-in Administrator",
			// Enabling the built-in Administrator on a deployed machine does
			// not make it Audit Mode; the completed state is authoritative.
			sig: func() platform.AuditSignals {
				s := realAuditMachine(stateComplete)
				s.HaveAuditBoot = false
				s.AuditBoot = 0
				return s
			}(),
			want: StateNormalInstalled,
		},
		{
			name: "normal installed with non-admin user and no audit signals",
			sig: platform.AuditSignals{
				SetupImageState:           stateComplete,
				HaveSetupImageState:       true,
				CurrentUserSID:            "S-1-5-21-1000-2000-3000-1013",
				CurrentUserIsBuiltinAdmin: false,
			},
			want: StateNormalInstalled,
		},
		{
			name: "normal installed with a leftover SetupComplete CmdLine",
			sig: platform.AuditSignals{
				SetupImageState:           stateComplete,
				HaveSetupImageState:       true,
				SetupCmdLine:              `C:\Windows\Setup\Scripts\SetupComplete.cmd`,
				HaveSetupCmdLine:          true,
				CurrentUserSID:            "S-1-5-21-1000-2000-3000-1013",
				CurrentUserIsBuiltinAdmin: false,
			},
			want: StateNormalInstalled,
		},

		// OOBE.
		{
			name: "live OOBE: UNDEPLOYABLE with OOBEInProgress",
			sig: platform.AuditSignals{
				SetupImageState:           stateUndeployable,
				HaveSetupImageState:       true,
				OOBEInProgress:            1,
				HaveOOBEInProgress:        true,
				CurrentUserSID:            "S-1-5-21-1000-2000-3000-1008",
				CurrentUserIsBuiltinAdmin: false,
			},
			want: StateOOBE,
		},
		{
			name: "pending OOBE: SPECIALIZE_RESEAL_TO_OOBE",
			sig: platform.AuditSignals{
				SetupImageState:     stateSpecializeResealOOBE,
				HaveSetupImageState: true,
			},
			want: StateOOBE,
		},
		{
			name: "pending OOBE: GENERALIZE_RESEAL_TO_OOBE",
			sig: platform.AuditSignals{
				SetupImageState:     stateGeneralizeResealOOBE,
				HaveSetupImageState: true,
			},
			want: StateOOBE,
		},

		// Transitional and contradictory states must not be guessed.
		{
			name: "audit signals but session user is not the built-in Administrator",
			sig: func() platform.AuditSignals {
				s := realAuditMachine(stateUndeployable)
				s.CurrentUserSID = "S-1-5-21-1000-2000-3000-1013"
				s.CurrentUserIsBuiltinAdmin = false
				return s
			}(),
			want: StateUncertain,
		},
		{
			name: "audit signals but session user SID unavailable",
			sig: func() platform.AuditSignals {
				s := realAuditMachine(stateUndeployable)
				s.CurrentUserSID = ""
				s.CurrentUserIsBuiltinAdmin = false
				return s
			}(),
			want: StateUncertain,
		},
		{
			name: "audit-family state with the audit boot flag missing",
			sig: func() platform.AuditSignals {
				s := realAuditMachine(stateUndeployable)
				s.HaveAuditBoot = false
				s.AuditBoot = 0
				return s
			}(),
			want: StateUncertain,
		},
		{
			name: "audit-family state with the audit boot flag explicitly zero",
			sig: func() platform.AuditSignals {
				s := realAuditMachine(stateUndeployable)
				s.AuditBoot = 0
				return s
			}(),
			want: StateUncertain,
		},
		{
			name: "completed state conflicts with the audit boot flag",
			sig: func() platform.AuditSignals {
				s := realAuditMachine(stateComplete)
				return s
			}(),
			want: StateUncertain,
		},
		{
			name: "completed state conflicts with live OOBE",
			sig: platform.AuditSignals{
				SetupImageState:     stateComplete,
				HaveSetupImageState: true,
				OOBEInProgress:      1,
				HaveOOBEInProgress:  true,
			},
			want: StateUncertain,
		},
		{
			name: "RESEAL_TO_AUDIT while OOBE is running is a conflict",
			sig: func() platform.AuditSignals {
				s := realAuditMachine(stateSpecializeResealAudit)
				s.OOBEInProgress = 1
				return s
			}(),
			want: StateUncertain,
		},
		{
			name: "RESEAL_TO_OOBE conflicts with the audit boot flag",
			sig: platform.AuditSignals{
				SetupImageState:     stateSpecializeResealOOBE,
				HaveSetupImageState: true,
				AuditBoot:           1,
				HaveAuditBoot:       true,
			},
			want: StateUncertain,
		},
		{
			name: "registry ImageState and StateName disagree",
			sig: func() platform.AuditSignals {
				s := realAuditMachine(stateUndeployable)
				s.SetupStateName, s.HaveSetupStateName = stateComplete, true
				return s
			}(),
			want: StateUncertain,
		},
		{
			name: "registry ImageState and State.ini disagree",
			sig: func() platform.AuditSignals {
				s := realAuditMachine(stateUndeployable)
				s.StateIniImageState, s.HaveStateIniImageState = stateComplete, true
				return s
			}(),
			want: StateUncertain,
		},
		{
			name: "undeployable with no corroborating signals at all",
			sig: platform.AuditSignals{
				SetupImageState:     stateUndeployable,
				HaveSetupImageState: true,
			},
			want: StateUncertain,
		},

		// Unreadable and unrecognized states.
		{
			name: "state missing entirely",
			sig:  platform.AuditSignals{},
			want: StateUnknown,
		},
		{
			name: "unrecognized state value",
			sig: platform.AuditSignals{
				SetupImageState:     "IMAGE_STATE_CUSTOM_WEIRD",
				HaveSetupImageState: true,
			},
			want: StateUnknown,
		},
		{
			name: "StateName fallback when ImageState is absent",
			sig: func() platform.AuditSignals {
				s := realAuditMachine(stateUndeployable)
				s.SetupImageState, s.HaveSetupImageState = "", false
				s.SetupStateName, s.HaveSetupStateName = stateUndeployable, true
				return s
			}(),
			want: StateAuditMode,
		},
		{
			name: "State.ini fallback when both registry values are absent",
			sig: func() platform.AuditSignals {
				s := realAuditMachine(stateUndeployable)
				s.SetupImageState, s.HaveSetupImageState = "", false
				s.StateIniImageState, s.HaveStateIniImageState = stateUndeployable, true
				return s
			}(),
			want: StateAuditMode,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Evaluate(tt.sig)
			if got.State != tt.want {
				t.Fatalf("Evaluate() state = %v, want %v (summary: %s)", got.State, tt.want, got.Summary)
			}
			if got.Summary == "" {
				t.Error("Evaluate() produced an empty summary; every state needs an explanation")
			}
			if len(got.Signals) == 0 {
				t.Error("Evaluate() recorded no signals; assessments must show what was observed")
			}
		})
	}
}

// TestUncertainSummariesNameTheFailingCheck verifies that an uncertain
// verdict explains which observation prevented confirmation, so an operator
// on a rejected machine can act on the report instead of guessing.
func TestUncertainSummariesNameTheFailingCheck(t *testing.T) {
	tests := []struct {
		name string
		sig  platform.AuditSignals
		want string
	}{
		{
			name: "non-admin session",
			sig: func() platform.AuditSignals {
				s := realAuditMachine(stateUndeployable)
				s.CurrentUserSID = "S-1-5-21-1000-2000-3000-1013"
				s.CurrentUserIsBuiltinAdmin = false
				return s
			}(),
			want: "built-in Administrator",
		},
		{
			name: "missing audit boot flag",
			sig: func() platform.AuditSignals {
				s := realAuditMachine(stateUndeployable)
				s.HaveAuditBoot = false
				s.AuditBoot = 0
				return s
			}(),
			want: "AuditBoot",
		},
		{
			name: "unreadable user",
			sig: func() platform.AuditSignals {
				s := realAuditMachine(stateUndeployable)
				s.CurrentUserSID = ""
				s.CurrentUserIsBuiltinAdmin = false
				return s
			}(),
			want: "session user",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Evaluate(tt.sig)
			if got.State != StateUncertain {
				t.Fatalf("state = %v, want StateUncertain", got.State)
			}
			if !containsFold(got.Summary, tt.want) {
				t.Errorf("summary does not name the failing check %q:\n%s", tt.want, got.Summary)
			}
		})
	}
}

func containsFold(s, sub string) bool {
	n, m := len(s), len(sub)
	for i := 0; i+m <= n; i++ {
		if equalFold(s[i:i+m], sub) {
			return true
		}
	}
	return false
}

func equalFold(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		ca, cb := a[i], b[i]
		if 'A' <= ca && ca <= 'Z' {
			ca += 'a' - 'A'
		}
		if 'A' <= cb && cb <= 'Z' {
			cb += 'a' - 'A'
		}
		if ca != cb {
			return false
		}
	}
	return true
}

func TestParseStateIni(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
		wantOK  bool
	}{
		{
			name:    "documented shape with CRLF and quoted value",
			content: "[State]\r\nImageState=\"IMAGE_STATE_COMPLETE\"\r\n",
			want:    "IMAGE_STATE_COMPLETE",
			wantOK:  true,
		},
		{
			name:    "unquoted value",
			content: "[State]\nImageState=IMAGE_STATE_UNDEPLOYABLE\n",
			want:    "IMAGE_STATE_UNDEPLOYABLE",
			wantOK:  true,
		},
		{
			name:    "case-insensitive section and key",
			content: "[state]\nimagestate=\"IMAGE_STATE_COMPLETE\"\n",
			want:    "IMAGE_STATE_COMPLETE",
			wantOK:  true,
		},
		{
			name:    "StateName key accepted like the registry fallback",
			content: "[State]\nStateName=\"IMAGE_STATE_UNDEPLOYABLE\"\n",
			want:    "IMAGE_STATE_UNDEPLOYABLE",
			wantOK:  true,
		},
		{
			name:    "other sections ignored",
			content: "[Other]\nImageState=\"X\"\n[State]\nImageState=\"IMAGE_STATE_COMPLETE\"\n",
			want:    "IMAGE_STATE_COMPLETE",
			wantOK:  true,
		},
		{
			name:    "comment lines ignored",
			content: "; setup wrote this\n[State]\n; remark\nImageState=\"IMAGE_STATE_COMPLETE\"\n",
			want:    "IMAGE_STATE_COMPLETE",
			wantOK:  true,
		},
		{
			name:    "missing section",
			content: "ImageState=\"IMAGE_STATE_COMPLETE\"\n",
			want:    "",
			wantOK:  false,
		},
		{
			name:    "corrupt file",
			content: "\x00\x01garbage\n\x02\x03",
			want:    "",
			wantOK:  false,
		},
		{
			name:    "empty file",
			content: "",
			want:    "",
			wantOK:  false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := parseStateIni(tt.content)
			if got != tt.want || ok != tt.wantOK {
				t.Errorf("parseStateIni() = (%q, %v), want (%q, %v)", got, ok, tt.want, tt.wantOK)
			}
		})
	}
}

func TestCollectReportsMissingValuesAsAbsent(t *testing.T) {
	reg := fake.NewRegistry()
	sys := &fakeSystem{usr: adminUser()}
	svc := NewService(reg, sys, nil)
	sig, err := svc.Collect(t.Context())
	if err != nil {
		t.Fatalf("Collect() error = %v", err)
	}
	if sig.HaveSetupImageState {
		t.Error("HaveSetupImageState should be false when the value is absent")
	}
	if sig.HaveSetupStateName {
		t.Error("HaveSetupStateName should be false when the value is absent")
	}
	if sig.HaveSetupCmdLine {
		t.Error("HaveSetupCmdLine should be false when the value is absent")
	}
	if sig.HaveAuditBoot {
		t.Error("HaveAuditBoot should be false when the value is absent")
	}
	if sig.CurrentUserSID != "S-1-5-21-1-2-3-500" {
		t.Errorf("CurrentUserSID = %q, want the fake SID", sig.CurrentUserSID)
	}
	if !sig.CurrentUserIsBuiltinAdmin {
		t.Error("CurrentUserIsBuiltinAdmin should follow the fake user")
	}
}

func TestCollectSurfacesReadFailures(t *testing.T) {
	reg := newFailingRegistry()
	sys := &fakeSystem{}
	svc := NewService(reg, sys, nil)
	if _, err := svc.Collect(t.Context()); err == nil {
		t.Fatal("Collect() should return an error when the registry is unreadable")
	}
}

// TestCollectReadsEverySignal verifies the collector actually reads all the
// documented locations: a broken read of any one of them must show up as an
// error, and a present value must land in the struct.
func TestCollectReadsEverySignal(t *testing.T) {
	reg := fake.NewRegistry().
		SetString(platform.HKLM, `SOFTWARE\Microsoft\Windows\CurrentVersion\Setup\State`, "ImageState", stateUndeployable).
		SetDWORD(platform.HKLM, `SYSTEM\Setup\Status`, "AuditBoot", 1).
		SetDWORD(platform.HKLM, `SYSTEM\Setup\Status`, "AuditBootVolatile", 0).
		SetString(platform.HKLM, `SYSTEM\Setup`, "CmdLine", "oobe\\windeploy.exe").
		SetDWORD(platform.HKLM, `SYSTEM\Setup`, "OOBEInProgress", 0).
		SetDWORD(platform.HKLM, `SYSTEM\Setup`, "SystemSetupInProgress", 0)
	sys := &fakeSystem{usr: adminUser()}
	ini := &fake.SetupStateFile{
		Content: "[State]\r\nImageState=\"IMAGE_STATE_UNDEPLOYABLE\"\r\n",
		Exists:  true,
	}
	svc := NewService(reg, sys, ini)

	sig, err := svc.Collect(t.Context())
	if err != nil {
		t.Fatalf("Collect() error = %v", err)
	}
	if !sig.HaveSetupImageState || sig.SetupImageState != stateUndeployable {
		t.Errorf("SetupImageState = (%q, %v), want (%q, true)", sig.SetupImageState, sig.HaveSetupImageState, stateUndeployable)
	}
	if !sig.HaveAuditBoot || sig.AuditBoot != 1 {
		t.Errorf("AuditBoot = (%d, %v), want (1, true)", sig.AuditBoot, sig.HaveAuditBoot)
	}
	if !sig.HaveAuditBootVolatile || sig.AuditBootVolatile != 0 {
		t.Errorf("AuditBootVolatile = (%d, %v), want (0, true)", sig.AuditBootVolatile, sig.HaveAuditBootVolatile)
	}
	if !sig.HaveSetupCmdLine || sig.SetupCmdLine != "oobe\\windeploy.exe" {
		t.Errorf("SetupCmdLine = (%q, %v), want the windeploy value", sig.SetupCmdLine, sig.HaveSetupCmdLine)
	}
	if !sig.HaveOOBEInProgress || sig.OOBEInProgress != 0 {
		t.Errorf("OOBEInProgress = (%d, %v), want (0, true)", sig.OOBEInProgress, sig.HaveOOBEInProgress)
	}
	if !sig.HaveSystemSetupInProgress || sig.SystemSetupInProgress != 0 {
		t.Errorf("SystemSetupInProgress = (%d, %v), want (0, true)", sig.SystemSetupInProgress, sig.HaveSystemSetupInProgress)
	}
	if !sig.HaveStateIniImageState || sig.StateIniImageState != stateUndeployable {
		t.Errorf("StateIniImageState = (%q, %v), want (%q, true)", sig.StateIniImageState, sig.HaveStateIniImageState, stateUndeployable)
	}
	if sig.StateIniErr != nil {
		t.Errorf("StateIniErr = %v, want nil", sig.StateIniErr)
	}

	a := Evaluate(sig)
	if a.State != StateAuditMode {
		t.Fatalf("Evaluate() state = %v, want StateAuditMode (summary: %s)", a.State, a.Summary)
	}
}

// TestCollectRecordsStateIniReadError checks that a State.ini read failure is
// recorded (and reported in the unknown-state summary) rather than silently
// ignored or treated as a missing file.
func TestCollectRecordsStateIniReadError(t *testing.T) {
	reg := fake.NewRegistry()
	sys := &fakeSystem{usr: adminUser()}
	ini := &fake.SetupStateFile{Err: errors.New("access denied")}
	svc := NewService(reg, sys, ini)

	sig, err := svc.Collect(t.Context())
	if err != nil {
		t.Fatalf("Collect() should not fail when only State.ini is unreadable: %v", err)
	}
	if sig.StateIniErr == nil {
		t.Fatal("StateIniErr should record the read failure")
	}
	a := Evaluate(sig)
	if a.State != StateUnknown {
		t.Fatalf("Evaluate() state = %v, want StateUnknown", a.State)
	}
	if !containsFold(a.Summary, "State.ini") {
		t.Errorf("summary should mention the State.ini failure:\n%s", a.Summary)
	}
}

func TestStateStrings(t *testing.T) {
	cases := map[State]string{
		StateAuditMode:       "Audit Mode (confirmed)",
		StateOOBE:            "First-time setup (OOBE) in progress",
		StateNormalInstalled: "Normal Windows installation (setup complete)",
		StateUncertain:       "Setup state uncertain",
		StateUnknown:         "Setup state unknown",
	}
	for state, want := range cases {
		if got := state.String(); got != want {
			t.Errorf("State(%d).String() = %q, want %q", state, got, want)
		}
		if got := state.Short(); got == "" {
			t.Errorf("State(%d).Short() is empty", state)
		}
	}
}
