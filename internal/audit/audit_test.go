package audit

import (
	"testing"

	"github.com/developerasaad/winprep/internal/platform"
	"github.com/developerasaad/winprep/internal/platform/fake"
)

func signals(state string, haveState bool, cmd string, haveCmd bool) platform.AuditSignals {
	return platform.AuditSignals{
		SetupStateName:            state,
		HaveSetupStateName:        haveState,
		SetupCmdLine:              cmd,
		HaveSetupCmdLine:          haveCmd,
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
		{
			name: "audit mode, all signals consistent",
			sig:  signals(stateUndeployable, true, cmdAudit, true),
			want: StateAuditMode,
		},
		{
			name: "audit mode cmd with different case",
			sig:  signals(stateUndeployable, true, "Audit.exe", true),
			want: StateAuditMode,
		},
		{
			name: "audit mode cmd with surrounding spaces",
			sig:  signals(stateUndeployable, true, " audit.exe ", true),
			want: StateAuditMode,
		},
		{
			name: "audit signals but session user is not builtin admin",
			sig: func() platform.AuditSignals {
				s := signals(stateUndeployable, true, cmdAudit, true)
				s.CurrentUserIsBuiltinAdmin = false
				s.CurrentUserSID = "S-1-5-21-1000-2000-3000-1013"
				return s
			}(),
			want: StateUncertain,
		},
		{
			name: "audit signals but session user sid unavailable",
			sig: func() platform.AuditSignals {
				s := signals(stateUndeployable, true, cmdAudit, true)
				s.CurrentUserSID = ""
				s.CurrentUserIsBuiltinAdmin = false
				return s
			}(),
			want: StateUncertain,
		},
		{
			name: "oobe via cmdline",
			sig:  signals(stateUndeployable, true, cmdOOBE, true),
			want: StateOOBE,
		},
		{
			name: "oobe via OOBEInProgress without cmdline",
			sig: func() platform.AuditSignals {
				s := signals(stateUndeployable, true, "", false)
				s.HaveOOBEInProgress = true
				s.OOBEInProgress = 1
				return s
			}(),
			want: StateOOBE,
		},
		{
			name: "undeployable with no other signals",
			sig:  signals(stateUndeployable, true, "", false),
			want: StateUncertain,
		},
		{
			name: "undeployable with cmd value absent",
			sig: func() platform.AuditSignals {
				s := signals(stateUndeployable, true, "", false)
				s.HaveSystemSetupInProgress = true
				return s
			}(),
			want: StateUncertain,
		},
		{
			name: "normal installed",
			sig:  signals(stateComplete, true, "", false),
			want: StateNormalInstalled,
		},
		{
			name: "normal installed with empty cmdline value present",
			sig:  signals(stateComplete, true, "", true),
			want: StateNormalInstalled,
		},
		{
			name: "complete state conflicts with audit cmdline",
			sig:  signals(stateComplete, true, cmdAudit, true),
			want: StateUncertain,
		},
		{
			name: "complete state with oobe cmdline treated as normal",
			// After OOBE completes, CmdLine can retain oobe.exe on some
			// builds; the completed StateName is authoritative there.
			sig:  signals(stateComplete, true, cmdOOBE, true),
			want: StateNormalInstalled,
		},
		{
			name: "state name missing entirely",
			sig:  signals("", false, "", false),
			want: StateUnknown,
		},
		{
			name: "unrecognized state name",
			sig:  signals("IMAGE_STATE_CUSTOM_WEIRD", true, "", false),
			want: StateUnknown,
		},
		{
			name: "audit cmdline present without state name",
			sig:  signals("", false, cmdAudit, true),
			want: StateUnknown,
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

func TestCollectReportsMissingValuesAsAbsent(t *testing.T) {
	reg := fake.NewRegistry()
	sys := &fakeSystem{usr: adminUser()}
	svc := NewService(reg, sys)
	sig, err := svc.Collect(t.Context())
	if err != nil {
		t.Fatalf("Collect() error = %v", err)
	}
	if sig.HaveSetupStateName {
		t.Error("HaveSetupStateName should be false when the value is absent")
	}
	if sig.HaveSetupCmdLine {
		t.Error("HaveSetupCmdLine should be false when the value is absent")
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
	svc := NewService(reg, sys)
	if _, err := svc.Collect(t.Context()); err == nil {
		t.Fatal("Collect() should return an error when the registry is unreadable")
	}
}
