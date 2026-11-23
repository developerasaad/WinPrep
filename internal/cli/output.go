package cli

import (
	"fmt"
	"runtime"
	"strings"

	"github.com/developerasaad/winprep/internal/app"
	"github.com/developerasaad/winprep/internal/textfile"
)

// usageText is the short command summary printed for usage errors.
func usageText() string {
	return strings.Join([]string{
		"Usage: winprep [command]",
		"",
		"Commands:",
		"  status    Show a read-only summary of this Windows installation",
		"  persist   Verify the laptop is ready for customer demonstrations",
		"  check     Run read-only hardware diagnostics",
		"  clean     Prepare the laptop for handover to a buyer (reseal into first-time setup)",
		"  help      Show full help",
		"  version   Show the version",
		"",
		"Run \"winprep help\" for details, or run winprep without a command for the menu.",
	}, "\n")
}

// helpText is the full help screen. It describes only what the tool actually
// does; see docs/workflow.md for the shop workflow and docs/safety.md for the
// security context.
func helpText(a *app.App) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s\n", app.VersionString())
	b.WriteString("A preparation and handover utility for Windows laptops being resold.\n")
	b.WriteString("\nUsage:\n")
	b.WriteString("  winprep                Open the interactive menu\n")
	b.WriteString("  winprep status         Read-only summary of this Windows installation\n")
	b.WriteString("  winprep persist        Verify the laptop is ready for demonstrations\n")
	b.WriteString("  winprep check          Read-only hardware diagnostics\n")
	b.WriteString("  winprep clean          Prepare the laptop for handover to a buyer\n")
	b.WriteString("  winprep clean --dry-run  Inspect only; report whether a handover is possible\n")
	b.WriteString("  winprep help           Show this help\n")
	b.WriteString("  winprep version        Show the version\n")
	b.WriteString("\nCommands:\n")
	b.WriteString("\n  status\n")
	b.WriteString("    Shows the Windows edition and build, the computer, the current user and\n")
	b.WriteString("    privilege level, the setup state (including Audit Mode detection), and\n")
	b.WriteString("    whether this machine fits WinPrep's workflow. Changes nothing.\n")
	b.WriteString("\n  persist\n")
	b.WriteString("    Verifies the machine is in a supported Audit Mode session and explains\n")
	b.WriteString("    what that means for demonstrations. Windows keeps Audit Mode alive across\n")
	b.WriteString("    restarts by itself, so this command does not modify the system.\n")
	b.WriteString("\n  check\n")
	b.WriteString("    Collects computer details, storage, battery, device problems, and the\n")
	b.WriteString("    Windows activation state. Read-only. Options:\n")
	b.WriteString("      --export <path>      Save the report to a file\n")
	b.WriteString("      --battery-report [path]\n")
	b.WriteString("                           Also generate a Windows battery report (HTML)\n")
	b.WriteString("\n  clean\n")
	b.WriteString("    Runs the full handover procedure: safety inspection, removal of\n")
	b.WriteString("    WinPrep's own artifacts, explicit confirmation, then\n")
	b.WriteString("    Sysprep.exe /oobe /shutdown to reseal Windows into first-time setup.\n")
	b.WriteString("    Requires an administrator console. Options:\n")
	b.WriteString("      --dry-run  Inspect and report only; change nothing\n")
	b.WriteString("      --yes      Skip the interactive prompt (safety checks still apply)\n")
	b.WriteString("\n")
	b.WriteString("    Important: resealing is not a factory reset and not a secure erase.\n")
	b.WriteString("    Accounts, files, and traces from preparation and customer testing remain\n")
	b.WriteString("    on the disk. Wipe the disk separately if the laptop's data must be gone.\n")
	b.WriteString("\nExit codes:\n")
	b.WriteString("  0  success (or: inspection-only command finished with findings shown)\n")
	b.WriteString("  1  the command failed or was declined\n")
	b.WriteString("  2  invalid usage\n")
	b.WriteString("\nWinPrep supports Windows 10 and Windows 11 (x64). It works offline and\n")
	b.WriteString("collects nothing. See docs/workflow.md, docs/safety.md, and\n")
	b.WriteString("docs/troubleshooting.md for details.\n")
	return b.String()
}

// buildTarget reports the GOOS/GOARCH this binary was built for, so a copy of
// winprep.exe built for the wrong platform says so honestly.
func buildTarget() string {
	return runtime.GOOS + "/" + runtime.GOARCH
}

// writeTextFile saves a report through the shared text writer (UTF-8 with
// Windows line endings).
func writeTextFile(path, content string) error {
	return textfile.Write(path, content)
}
