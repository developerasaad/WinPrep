// Package app wires the platform implementations and domain services into
// one application object. Both the CLI and the TUI consume the same App, so
// there is exactly one implementation of every operation.
package app

// Version is the WinPrep release version. It follows semantic versioning;
// prerelease labels (e.g. -alpha.1) mark builds whose full Windows lifecycle
// has not been validated yet.
const Version = "0.1.0-alpha.1"

// BuildCommit is set at build time via -ldflags. It is empty for local
// builds, which is reported honestly rather than guessed.
var BuildCommit = ""

// VersionString returns the human-readable version banner.
func VersionString() string {
	commit := BuildCommit
	if commit == "" {
		commit = "unknown commit"
	}
	return "winprep " + Version + " (" + commit + ")"
}
