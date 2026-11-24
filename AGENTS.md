# AGENTS.md

Rules for coding agents (and anyone automating work) on this repository.
These are constraints, not suggestions; a change that violates them is wrong
even if it works.

## Scope

WinPrep prepares Windows laptops for resale. It never: reinstalls Windows,
wipes or formats disks, removes accounts, installs drivers, touches
activation, disables Windows security features, adds telemetry or network
communication, or self-updates. If a task seems to require one of these,
stop and reconsider - the answer is usually that WinPrep is the wrong tool
for that job.

## Architecture boundaries

- All Windows access goes through `internal/platform` interfaces. Domain
  packages (`audit`, `handover`, `persist`) and presentation (`cli`, `tui`)
  import those interfaces, never `internal/windows`.
- Only `internal/app` assembles implementations into the `App`. The CLI and
  TUI share it; duplicate business logic in a presentation layer is a bug.
- Platform implementations stay read-only unless the operation is the
  handover itself: the approved cleanup of demonstration data, WinPrep's own
  artifacts, and the single documented Sysprep call. Deletions only ever
  touch paths the cleanup plan validated; reparse points are unlinked, never
  followed.

## Safety rules

- The handover preflight has no bypass. Never add a flag that skips
  blockers or unknown checks.
- Nothing is modified before the operator's confirmation, and the cleanup
  only ever runs the plan that was confirmed. A scope that changed since
  confirmation aborts rather than expands.
- `clean --dry-run` must not mutate anything. If you add a check to the
  preflight, verify it runs under dry-run and changes nothing.
- Never add code that kills or schedules killing of `sysprep.exe`, and
  never make the Sysprep invocation cancellable once accepted. WinPrep also
  never launches Sysprep while any sysprep.exe process exists.
- Never delete user accounts or profile folders through the cleanup; the
  handover blocks on them so the operator removes them through Windows.
- Confirmation flows default to "no". The TUI confirm selector starts on
  the safe choice; CLI prompts treat empty/invalid input as decline.
- Errors surface as errors. No fabricated success messages, no swallowed
  failures behind generic completion text, no "verified" claims for
  anything not actually tested.
- External commands use explicit argument vectors. No shell strings, no
  `irm | iex` patterns, no downloaded scripts.

## Detection honesty

Audit Mode detection reads the documented Windows setup state
(`ImageState`, with the State.ini file and the documented-but-unobserved
`StateName` value as fallbacks), the audit boot flag
(`HKLM\SYSTEM\Setup\Status\AuditBoot`, field-observed rather than
officially documented), the OOBE indicator, and the session SID, and
confirms Audit Mode only when they agree. Do not weaken it to "make
detection work" on some machine; uncertain states must stay uncertain and
must disable state-changing operations. Never treat a single registry value
as proof, and never gate detection on `HKLM\SYSTEM\Setup\CmdLine` - real
systems use that value for unrelated pre-logon commands, so it is recorded
as an observation only. A machine rejected despite a genuine Audit Mode
session is a bug to be reported with `winprep signals` output, not a reason
to relax the rules.

## Testing

- New behavior ships with unit tests against the fakes in
  `internal/platform/fake`, in the package it belongs to.
- Tests never invoke real Sysprep, modify a real registry, or depend on a
  particular Windows edition. Anything needing a real Windows installation
  goes in `tests/windows` with a `//go:build windows` tag.
- `gofmt -l .` must be empty, `go vet ./...` and `go test -race ./...` must
  pass, and the project must cross-compile for windows/amd64 before you
  call anything done.
- Do not mark a test as covering behavior it did not exercise. If
  something cannot be tested in the available environment, say so in the
  PR description and docs.

## Writing

User-facing strings, docs, commits, and release notes are plain technical
English. No marketing language, no emojis, no filler. Say exactly what
happened, what it means, and what to do next. Never claim a feature exists
that is not in the code, and never describe untested behavior as verified.

## Version and release

The version constant lives in `internal/app/version.go`; release tags must
match it. Version strings carry a prerelease label while the Windows
lifecycle test is outstanding - do not strip the label to "ship stable".
