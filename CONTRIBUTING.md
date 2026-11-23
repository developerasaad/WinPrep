# Contributing to WinPrep

Thanks for taking the time to contribute.

## Before you start

Read [README.md](README.md) - especially "What it does not do" - and
[docs/safety.md](docs/safety.md). WinPrep runs on machines that are about to
be sold to real people; scope and safety decisions in this project are
deliberate. A pull request that adds a feature the project excludes (disk
wiping, driver installation, security-feature toggles, telemetry) will be
declined even if the code is good.

## Bugs

Open an issue first for anything non-trivial. Include the WinPrep version,
the Windows edition and build, whether the console was elevated, and the
exact output. For handover failures, the Sysprep logs under
`%WINDIR%\System32\Sysprep\Panther` are usually decisive - see
[docs/troubleshooting.md](docs/troubleshooting.md).

Security problems go through private vulnerability reporting, never a
public issue: see [SECURITY.md](SECURITY.md).

## Pull requests

- Keep changes focused. A PR that fixes one bug or adds one feature is
  reviewable; a PR that does both is not.
- New behavior comes with unit tests. The fakes in
  `internal/platform/fake` exist so tests never need a real Windows box;
  see [docs/development.md](docs/development.md) for the layering rules.
- Windows-specific behavior that needs a real machine belongs in
  `tests/windows` (CI runs it on a Windows runner).
- Run `gofmt`, `go vet ./...`, and `go test ./...` before pushing. CI runs
  the same.
- Match the existing tone in user-facing strings: plain English, no
  marketing language, no false certainty. An error message that says what
  failed and what to do next is worth more than a clever one.
- Do not add dependencies without a concrete benefit; the dependency count
  is a feature.

## Commit style

Short imperative subject lines ("Add Audit Mode detection", not "added some
audit stuff" or "Update files"). The body explains *why* when the change is
not self-evident. There is no commit-message bot; just write like you would
want to read six months later.

## Conduct

Be decent. [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md) applies.
