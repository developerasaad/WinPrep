# Development

How to build, test, and maintain WinPrep. AGENTS.md carries the rules that
apply to any contributor (human or tooling); this document explains the
mechanics.

## Prerequisites

- Go 1.27 or newer (the version in `go.mod` is authoritative).
- No CGO dependencies; the Windows build cross-compiles from any OS.
- For Windows integration tests: a Windows machine with a Go toolchain.
  The GitHub Actions Windows runner covers this in CI.

## Layout

```
cmd/winprep/        executable entry point
internal/app/       assembly: wires platform implementations into services
internal/cli/       command-line interface
internal/tui/       interactive terminal interface + fallback menu
internal/platform/  interfaces and typed models (the seam for all testing)
internal/platform/fake/  in-memory fakes
internal/windows/   real Windows implementation (registry, tokens,
                    processes, SAM accounts, Sysprep monitor, mutex)
internal/audit/     Audit Mode detection
internal/persist/   "Prepare for Display"
internal/handover/  "Clean & Finish": the operation controller - preflight,
                    cleanup planning and execution, reseal, outcomes
internal/artifacts/ managed-artifact manifest
internal/format/    size formatting
internal/apptest/   full App over fakes, shared by cli/tui tests
tests/windows/      integration tests, run on real Windows (CI)
```

The dependency rule is one-directional: presentation (cli, tui) and domain
packages (audit, handover, persist) import platform interfaces; only
internal/windows implements them; only app assembles the two. If a change
needs a domain package to import internal/windows, the seam is being
bypassed and the change needs a second look.

The handover package owns one controller with a session-shaped API:
`Begin` (inspect + plan, read-only), `Approve` (record the operator's
decision), `Execute` (revalidate, clean, verify, reseal). The CLI, the TUI,
and the fallback menu all drive that sequence; none of them may sequence
deletions themselves.

## Building

```
go build ./...                          # current platform
GOOS=windows GOARCH=amd64 go build ./cmd/winprep   # release-style build
```

The release build adds `-trimpath -ldflags "-s -w"` and injects the commit
with `-X .../internal/app.BuildCommit=<sha>`; see `.github/workflows/release.yml`.

## Testing

```
go test ./...            # unit tests, any OS, milliseconds
go test -race ./...      # as CI runs them
```

On Windows, the same command also picks up `tests/windows` (or set
`WINPREP_EXE` to test a specific executable). The three test layers and
what each can prove are described in [tests/README.md](../tests/README.md).

Adding code: new behavior gets unit tests over the fakes, next to the
package it belongs to. A test that needs the real registry belongs in
`tests/windows`, not in `internal/`.

## Lifecycle testing (not yet executed)

The full workflow needs a disposable Windows VM with snapshots:

1. Install Windows 10 or 11; reach OOBE.
2. Press Ctrl+Shift+F3; confirm the machine restarts into Audit Mode.
3. Run `winprep persist`; confirm Audit Mode detection and the readiness
   explanation.
4. Restart; confirm the machine is still in Audit Mode.
5. Create demonstration data in the Administrator's Downloads folder, use
   Edge to browse somewhere, and leave files in the local Temp folder.
6. Run `winprep clean --dry-run`; confirm the plan lists exactly the
   created data and that nothing changed.
7. Create an extra local account; confirm the handover is blocked until it
   is removed.
8. Run `winprep clean`; confirm the plan and confirmation flow, then let
   the cleanup remove the demonstration data and Sysprep reseal and shut
   down.
9. Power on; confirm Windows first-time setup appears and the OOBE flow
   completes.
10. Inspect the disk (mount the image or use a second OS): the
    demonstration Downloads contents, Temp files, and Edge data are gone;
    `%ProgramData%\WinPrep` is gone; Desktop contents and other personal
    folders are untouched; no WinPrep startup helpers exist anywhere.

Failure cases to cover along the way: `clean` unelevated (blocked), a
second `persist` run (idempotent), `clean` with a pending reboot (blocked),
`clean` with a browser running (blocked), the System Preparation Tool
window open (attention, then the execution gate), declining at the
confirmation (nothing modified), interrupted inspection (ESC), and
narrow/odd terminal sizes.

As of v0.2.0-alpha.1 this list has not been executed on a reference
machine; the prerelease label reflects that. Record actual results here
when the pass happens - observed behavior, not expectations.

## Versioning

`internal/app/version.go` carries the version constant; tags follow
`vMAJOR.MINOR.PATCH` with prerelease labels (for example
`v0.1.0-alpha.1`) while Windows lifecycle testing is outstanding. The
release workflow refuses tags that do not match the constant.

## Releasing

See [releasing.md](releasing.md) for the procedure and the checks the
workflow performs.
