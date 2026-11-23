package status

import (
	"strings"
	"testing"

	"github.com/developerasaad/winprep/internal/audit"
	"github.com/developerasaad/winprep/internal/platform"
	"github.com/developerasaad/winprep/internal/platform/fake"
)

func newService(reg *fake.Registry, info platform.SystemInfo, usr platform.UserInfo) *Service {
	sys := &fake.System{Sys: info, Usr: usr}
	return NewService(sys, audit.NewService(reg, sys))
}

func auditRegistry() *fake.Registry {
	return fake.NewRegistry().
		SetString(platform.HKLM, `SOFTWARE\Microsoft\Windows\CurrentVersion\Setup\State`, "StateName", "IMAGE_STATE_UNDEPLOYABLE").
		SetString(platform.HKLM, `SYSTEM\Setup`, "CmdLine", "audit.exe")
}

func win11(info platform.SystemInfo) platform.SystemInfo {
	info.OSName = "Windows 11 Pro"
	info.Build = "10.0.22631.4317"
	info.InstallationType = "Client"
	info.Architecture = "x64"
	info.IsWindows = true
	return info
}

func TestAuditModeMachineIsSuitable(t *testing.T) {
	svc := newService(auditRegistry(), win11(platform.SystemInfo{
		Manufacturer: "LENOVO", Model: "ThinkPad T480", CPU: "Intel i5", TotalRAMBytes: 16 << 30,
	}), platform.UserInfo{IsElevated: true, SID: "S-1-5-21-1-2-3-500", IsBuiltinAdmin: true, Name: `WKS\Administrator`})

	r, err := svc.Inspect(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if !r.Suitable {
		t.Fatalf("audit mode client machine should be suitable; blocking: %v", r.Blocking)
	}
	text := strings.Join(r.Lines(), "\n")
	for _, want := range []string{"Windows 11 Pro", "Audit Mode", "built-in Administrator", "elevated", "Suitable for the WinPrep workflow: Yes"} {
		if !strings.Contains(text, want) {
			t.Errorf("status output missing %q", want)
		}
	}
}

func TestNormalMachineIsNotSuitable(t *testing.T) {
	reg := fake.NewRegistry().
		SetString(platform.HKLM, `SOFTWARE\Microsoft\Windows\CurrentVersion\Setup\State`, "StateName", "IMAGE_STATE_COMPLETE")
	svc := newService(reg, win11(platform.SystemInfo{}), platform.UserInfo{})
	r, err := svc.Inspect(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if r.Suitable {
		t.Error("a deployed installation must not be reported suitable")
	}
	if len(r.Blocking) == 0 {
		t.Error("blocking conditions must be listed")
	}
}

func TestServerIsNotSuitable(t *testing.T) {
	svc := newService(auditRegistry(), win11(platform.SystemInfo{InstallationType: "Server"}), platform.UserInfo{})
	r, err := svc.Inspect(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if r.Suitable {
		t.Error("Windows Server must not be suitable")
	}
}

func TestUncertainStateDisablesWorkflow(t *testing.T) {
	svc := newService(fake.NewRegistry(), win11(platform.SystemInfo{}), platform.UserInfo{})
	r, err := svc.Inspect(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if r.Suitable {
		t.Error("uncertain state must disable the workflow")
	}
	if !strings.Contains(strings.Join(r.Lines(), "\n"), "cannot confirm Audit Mode") {
		t.Error("uncertainty must be explained in the output")
	}
}

func TestOOBEState(t *testing.T) {
	reg := fake.NewRegistry().
		SetString(platform.HKLM, `SOFTWARE\Microsoft\Windows\CurrentVersion\Setup\State`, "StateName", "IMAGE_STATE_UNDEPLOYABLE").
		SetString(platform.HKLM, `SYSTEM\Setup`, "CmdLine", "oobe.exe")
	svc := newService(reg, win11(platform.SystemInfo{}), platform.UserInfo{})
	r, err := svc.Inspect(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if r.Suitable {
		t.Error("OOBE state is not suitable for the workflow")
	}
	if !strings.Contains(strings.Join(r.Lines(), "\n"), "Ctrl+Shift+F3") {
		t.Error("OOBE blocking condition should point at Ctrl+Shift+F3")
	}
}

func TestMissingValuesRenderAsNotAvailable(t *testing.T) {
	svc := newService(auditRegistry(), win11(platform.SystemInfo{}), platform.UserInfo{})
	r, err := svc.Inspect(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	text := strings.Join(r.Lines(), "\n")
	if !strings.Contains(text, "(not available)") {
		t.Error("missing facts must render as not available, never as blank guesses")
	}
}
