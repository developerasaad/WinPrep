# Changelog

All notable changes to WinPrep are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and the project
uses semantic versioning.

## [Unreleased]

## [0.1.0-alpha.1] - 2026-11-23

First public build. The complete feature set of the initial version; the
alpha label reflects that the full Windows lifecycle (real Audit Mode
handover and OOBE verification on a reference machine) has not yet been
executed.

### Added

- Interactive terminal menu with keyboard navigation, async loading with
  ESC-cancel for read-only checks, and a plain numbered-menu fallback for
  restricted consoles.
- `winprep status`: read-only system summary with multi-signal Audit Mode
  detection (IMAGE_STATE_UNDEPLOYABLE + audit.exe command line + built-in
  Administrator session; conflicts evaluate to "uncertain", which disables
  state-changing operations).
- `winprep persist` ("Prepare for display"): verifies a supported Audit
  Mode session and explains restart persistence; makes no system changes.
- `winprep check`: read-only diagnostics - computer details, storage with
  health and the Windows disk identified via partition data, battery
  capacity/health with honest "unavailable" reporting, Device Manager
  problem devices, activation state; `--export` and `--battery-report`.
- `winprep clean` ("Ready for buyer"): seven-step handover - preflight
  inspection with ready/attention/blocked verdicts and no override,
  removal of WinPrep's own recorded artifacts, deliberate confirmation
  defaulting to cancel, `Sysprep.exe /oobe /shutdown`, and outcome-specific
  reporting with Panther log paths. `--dry-run` inspects without changes.
- Managed-artifact manifest under `%ProgramData%\WinPrep` with an
  ownership guard that refuses paths outside the WinPrep data directory.
- Unit tests over platform fakes, Windows integration tests on the CI
  runner, GitHub Actions CI, and a release workflow with checksums and
  build provenance attestation.

[Unreleased]: https://github.com/developerasaad/WinPrep/compare/v0.1.0-alpha.1...HEAD
[0.1.0-alpha.1]: https://github.com/developerasaad/WinPrep/releases/tag/v0.1.0-alpha.1
