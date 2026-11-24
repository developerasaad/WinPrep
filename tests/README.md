# Tests

WinPrep's validation is split into three layers. What each layer can and
cannot prove differs, and the split exists so nothing is claimed beyond what
was actually run.

## Unit tests (`go test ./...`)

Cross-platform, in `internal/*/`_test.go` next to the code they cover. All
Windows interaction goes through the fakes in `internal/platform/fake`, so
these tests run on any OS, execute in milliseconds, and never touch a real
registry, disk, or Sysprep. They cover audit-state evaluation, preflight
verdicts, cleanup planning and execution against real temporary directories
(including symlink and containment refusals), manifest handling, CLI
argument parsing and flows, TUI navigation and the confirmation gate, and
the fallback menu. The mandatory regression tests - cancellation modifies
nothing, the CLI and TUI drive the same deletions, the dry run is
read-only, cleanup failures block the reseal, an active Sysprep is never
overlapped, and a deployed installation is rejected - live here.

Run them with:

    go test ./...

## Windows integration tests (`tests/windows`)

Tagged `//go:build windows`; they run only on a real Windows installation:
the GitHub Actions Windows runner during CI, or a local Windows machine with
a Go toolchain. They exercise the real registry, token, account-enumeration,
and process plumbing (read-only) and run the built executable end-to-end:
version and help output, `persist` and `signals` output, dry-run refusal on
a deployed machine, execution from a path with spaces, and the no-terminal
guidance path. The real-machine audit test asserts that `ImageState` is
read - the regression guard for the v0.2.0-alpha.1 detector, which queried
a value name (`StateName`) that real Windows never writes.

CI points `WINPREP_EXE` at the executable it built. Without that variable
the tests build their own copy, so `go test ./...` on a Windows development
machine covers them too.

These tests deliberately never invoke Sysprep, reboot, or modify system
state. A GitHub-hosted runner is a disposable VM, but it is a normally
deployed Windows installation (Server edition), which makes it a useful
honesty check: WinPrep must recognize it as unsuitable for the handover
workflow and refuse. The smoke tests that assume a deployed machine also
skip themselves when the machine reports a confirmed Audit Mode session, so
running the suite on a real preparation machine can never trigger a
disposable `clean --yes` handover by accident.

## Windows lifecycle tests (manual)

The full workflow - install Windows, enter Audit Mode with Ctrl+Shift+F3,
run WinPrep, restart, verify persistence, create demonstration data, dry-run
the cleanup, real handover, confirm OOBE on next boot - requires a
disposable Windows installation with snapshots and cannot be automated on
ordinary hosted runners without violating WinPrep's own safety rules.

The procedure and a results checklist live in `docs/development.md`
(Lifecycle testing). As of v0.2.0-alpha.2 this layer has not been executed;
the prerelease label reflects exactly that.
