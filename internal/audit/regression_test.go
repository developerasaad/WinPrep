package audit

import (
	"testing"

	"github.com/developerasaad/winprep/internal/platform"
	"github.com/developerasaad/winprep/internal/platform/fake"
)

// TestDetectsRealWorldAuditModeMachine reproduces the reported defect: a
// genuine Audit Mode session as observed in the field (Windows 10 era,
// "trapped in audit mode" reports and OEM deployment logs) was rejected.
//
// What a real Audit Mode machine actually exposes:
//
//   - HKLM\SOFTWARE\Microsoft\Windows\CurrentVersion\Setup\State has a
//     REG_SZ value named ImageState (not StateName; the older Microsoft
//     documentation prose says StateName, but the page's own examples and
//     every real installation write ImageState). During an established
//     Audit Mode session it reads IMAGE_STATE_UNDEPLOYABLE.
//   - HKLM\SYSTEM\Setup\Status\AuditBoot is 1: the machine is configured to
//     keep booting into Audit Mode.
//   - HKLM\SYSTEM\Setup\CmdLine does not contain "audit.exe"; real systems
//     use that slot for oobe\windeploy.exe, SetupComplete.cmd, or leave it
//     empty. audit.exe is launched by the setup engine, not via CmdLine.
//   - The session user is the built-in Administrator (RID 500).
//
// The detector shipped in v0.2.0-alpha.1 read StateName (never present on
// real Windows) and required CmdLine to equal "audit.exe" (never observed
// on real Windows), so this machine evaluated to "unknown" and both
// operations were blocked. Written before the fix, this test failed with:
//
//	Inspect() state = Setup state unknown, want StateAuditMode
func TestDetectsRealWorldAuditModeMachine(t *testing.T) {
	reg := fake.NewRegistry().
		SetString(platform.HKLM, `SOFTWARE\Microsoft\Windows\CurrentVersion\Setup\State`, "ImageState", "IMAGE_STATE_UNDEPLOYABLE").
		SetDWORD(platform.HKLM, `SYSTEM\Setup\Status`, "AuditBoot", 1).
		SetString(platform.HKLM, `SYSTEM\Setup`, "CmdLine", "").
		SetDWORD(platform.HKLM, `SYSTEM\Setup`, "OOBEInProgress", 0).
		SetDWORD(platform.HKLM, `SYSTEM\Setup`, "SystemSetupInProgress", 0)
	ini := &fake.SetupStateFile{
		Content: "[State]\r\nImageState=\"IMAGE_STATE_UNDEPLOYABLE\"\r\n",
		Exists:  true,
	}
	sys := &fakeSystem{usr: adminUser()}
	svc := NewService(reg, sys, ini)

	assessment, err := svc.Inspect(t.Context())
	if err != nil {
		t.Fatalf("Inspect() error = %v", err)
	}
	if assessment.State != StateAuditMode {
		t.Fatalf("Inspect() state = %v (%s), want StateAuditMode.\nSummary: %s",
			assessment.State, assessment.State, assessment.Summary)
	}
}
