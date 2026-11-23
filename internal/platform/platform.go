// Package platform defines the interfaces and typed models that separate
// WinPrep's business logic from the operating system. Presentation code (CLI
// and TUI) and domain packages (audit, diagnostics, handover) work against
// these interfaces only; the Windows-specific implementations live in
// internal/windows. In-memory fakes for tests live in internal/platform/fake.
package platform

import (
	"context"
	"errors"
)

// ErrUnsupportedPlatform is returned by services when WinPrep is running on an
// operating system it cannot inspect. The Windows implementation is the only
// real one; other builds exist so the tool can be developed and unit-tested
// cross-platform.
var ErrUnsupportedPlatform = errors.New("winprep: this build does not support the current operating system; WinPrep runs on Windows 10/11 x64")

// RootKey identifies a predefined registry root key. Values mirror the usual
// HKEY_* constants without importing the Windows API into this package.
type RootKey uint32

const (
	HKLM RootKey = iota + 1 // HKEY_LOCAL_MACHINE
	HKU                     // HKEY_USERS
)

func (r RootKey) String() string {
	switch r {
	case HKLM:
		return "HKLM"
	case HKU:
		return "HKU"
	default:
		return "unknown"
	}
}

// ErrValueNotFound reports that a registry value or key does not exist. It is
// distinct from other read errors (such as access denied), which callers must
// not silently treat as "missing".
var ErrValueNotFound = errors.New("registry value not found")

// Registry provides read-only access to the registry values WinPrep inspects.
// Implementations must return ErrValueNotFound for absent values and keys, and
// a non-nil error for anything that could not be read reliably.
type Registry interface {
	// StringValue reads a REG_SZ or REG_EXPAND_SZ value. ok is false when the
	// value does not exist.
	StringValue(root RootKey, path, name string) (value string, ok bool, err error)
	// DWORDValue reads a REG_DWORD value. ok is false when the value does not
	// exist.
	DWORDValue(root RootKey, path, name string) (value uint32, ok bool, err error)
	// MultiStringValue reads a REG_MULTI_SZ value. ok is false when the value
	// does not exist.
	MultiStringValue(root RootKey, path, name string) (value []string, ok bool, err error)
	// KeyExists reports whether a key exists. An unreadable key returns an
	// error rather than false.
	KeyExists(root RootKey, path string) (bool, error)
	// SubKeyNames lists the names of a key's immediate subkeys.
	SubKeyNames(root RootKey, path string) ([]string, error)
}

// CommandResult holds the captured output of an external command that has
// finished (or been cancelled). A non-zero ExitCode is not itself an error;
// callers interpret it.
type CommandResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

// Runner executes external programs using explicit argument arrays. There is
// deliberately no shell involved: arguments are passed verbatim, which rules
// out injection through paths or other user-supplied values.
type Runner interface {
	// Run executes a command with its console window hidden (where the
	// platform distinguishes) and captures combined output. It is intended for
	// helper programs such as powershell.exe whose output is parsed.
	Run(ctx context.Context, name string, args ...string) (CommandResult, error)
	// RunVisible executes a command that is allowed to show windows and
	// interact with the user, such as the Sysprep GUI.
	RunVisible(ctx context.Context, name string, args ...string) (CommandResult, error)
}

// SystemInfo carries the fast, targeted system facts WinPrep shows in the main
// menu header and the status command. It is assembled from registry reads and
// native calls only, so it can be collected without spawning processes.
type SystemInfo struct {
	OSName           string // e.g. "Windows 11 Pro"
	Edition          string // e.g. "Pro"
	DisplayVersion   string // e.g. "23H2" (may be empty on old builds)
	Build            string // e.g. "10.0.22631.4317"
	InstallationType string // "Client", "Server", or "" when unknown
	Manufacturer     string
	Model            string
	CPU              string
	TotalRAMBytes    uint64
	Architecture     string // runtime.GOARCH of this build
	IsWindows        bool
}

// IsServer reports whether the system identifies itself as Windows Server.
func (s SystemInfo) IsServer() bool {
	return s.InstallationType == "Server"
}

// UserInfo describes the interactive user running WinPrep.
type UserInfo struct {
	Name           string // e.g. "WKS\Administrator"
	SID            string // SDDL form, e.g. S-1-5-21-...-500
	IsElevated     bool   // process token is elevated (UAC)
	IsBuiltinAdmin bool   // SID ends in -500
}

// System collects basic system and user facts using targeted queries.
type System interface {
	SystemInfo(ctx context.Context) (SystemInfo, error)
	UserInfo(ctx context.Context) (UserInfo, error)
}

// ProcessLister reports the names of running processes. WinPrep uses it to
// detect a running Sysprep instance; it must not spawn external tools.
type ProcessLister interface {
	ProcessNames(ctx context.Context) ([]string, error)
}

// AuditSignals holds the raw Windows setup-state values used to determine the
// deployment mode. Detection is intentionally multi-signal: no single value is
// trusted on its own. See internal/audit for the evaluation rules and the
// Microsoft references behind them.
type AuditSignals struct {
	// SetupStateName is HKLM\SOFTWARE\Microsoft\Windows\CurrentVersion\Setup\State\StateName.
	// "IMAGE_STATE_COMPLETE" means Windows setup finished; "IMAGE_STATE_UNDEPLOYABLE"
	// means the image is mid-deployment (Audit Mode or OOBE).
	SetupStateName     string
	HaveSetupStateName bool
	// SetupCmdLine is HKLM\SYSTEM\Setup\CmdLine. It is "audit.exe" in Audit
	// Mode and "oobe.exe" while OOBE runs.
	SetupCmdLine     string
	HaveSetupCmdLine bool
	// SystemSetupInProgress is HKLM\SYSTEM\Setup\SystemSetupInProgress (1 while
	// setup/audit is active).
	SystemSetupInProgress     uint32
	HaveSystemSetupInProgress bool
	// OOBEInProgress is HKLM\SYSTEM\Setup\OOBEInProgress (1 while OOBE runs).
	OOBEInProgress     uint32
	HaveOOBEInProgress bool
	// User context at the time of collection.
	CurrentUserSID            string
	CurrentUserIsBuiltinAdmin bool
}
