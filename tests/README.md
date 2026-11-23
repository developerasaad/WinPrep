# Tests

WinPrep's validation is split into three layers. What each layer can and
cannot prove differs, and the split exists so nothing is claimed beyond what
was actually run.

## Unit tests (`go test ./...`)

Cross-platform, in `internal/*/`_test.go` next to the code they cover. All
Windows interaction goes through the fakes in `internal/platform/fake`, so
these tests run on any OS, execute in milliseconds, and never touch a real
registry, disk, or Sysprep. They cover audit-state evaluation, preflight
verdicts, diagnostic parsing and rendering, manifest handling, CLI argument
parsing and flows, TUI navigation and the confirmation gate, and the fallback
menu.

Run them with:

    go test ./...

## Windows integration tests (`tests/windows`)

Tagged `//go:build windows`; they run only on a real Windows installation:
the GitHub Actions Windows runner during CI, or a local Windows machine with
a Go toolchain. They exercise the real registry, token, and process plumbing
(read-only) and run the built executable end-to-end: version and help
output, honest status facts, dry-run refusal on a deployed machine, execution
from a path with spaces, and the no-terminal guidance path.

CI points `WINPREP_EXE` at the executable it built. Without that variable
the tests build their own copy, so `go test ./...` on a Windows development
machine covers them too.

These tests deliberately never invoke Sysprep, reboot, or modify system
state. A GitHub-hosted runner is a disposable VM, but it is a normally
deployed Windows installation (Server edition), which makes it a useful
honesty check: WinPrep must recognize it as unsuitable for the handover
workflow and refuse.

## Windows lifecycle tests (manual)

The full workflow - install Windows, enter Audit Mode with Ctrl+Shift+F3,
run WinPrep, restart, verify persistence, dry-run, real handover, confirm
OOBE on next boot - requires a disposable Windows installation with
snapshots and cannot be automated on ordinary hosted runners without
violating WinPrep's own safety rules.

The procedure and a results checklist live in `docs/development.md`
(Lifecycle testing). As of v0.1.0-alpha.1 this layer has not been executed;
the prerelease label reflects exactly that.
