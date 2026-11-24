# Changelog

All notable changes to WinPrep are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and the project
uses semantic versioning.

## [Unreleased]

## [0.2.0-alpha.1] - 2026-11-24

The product narrowed to its two core operations, and the handover grew the
cleanup it always needed. The version number moves to the 0.2.0 line because
the scope change removes previously documented commands; the alpha label
still reflects that the full Windows lifecycle (real Audit Mode handover and
OOBE verification on a reference machine) has not yet been executed.

### Removed

- `winprep check` and everything behind it: hardware, storage, battery,
  device, and activation inspection, report export, battery report
  generation. Hardware health checks never belonged to the preparation
  workflow; a refurbisher who needs them should run a dedicated tool.
- `winprep status` and the standalone system-information screens. The facts
  worth showing are contextual and remain in the menu header and the
  handover inspection.

### Changed

- Clean & Finish is sequenced by one shared controller used identically by
  the CLI, the TUI, and the fallback menu. The previous CLI and fallback
  removed WinPrep's artifacts before the final confirmation, so declining
  could leave the machine modified; nothing is modified before confirmation
  now, and the message says so truthfully.
- A running `sysprep.exe` process no longer blocks the inspection outright.
  The idle System Preparation Tool window that Audit Mode opens at sign-in
  is reported as an attention item, while freshly written Panther logs mark
  a genuine operation in progress, which blocks. WinPrep still never
  launches Sysprep while another Sysprep process exists.
- Extra local accounts and leftover user profiles block the handover
  instead of merely warning: they survive the reseal and would be handed to
  the buyer. Removal stays a manual, Windows-supported operation.
- Local accounts are enumerated through the SAM management APIs instead of
  a PowerShell subprocess, so the inspection no longer parses command
  output and launches no helper processes.

### Added

- A real cleanup subsystem. Clean & Finish now builds a typed plan from the
  demonstration profile (resolved by SID through ProfileList): the Downloads
  contents, the profile's temporary files, and the browsing data of Edge and
  Chrome, plus WinPrep's own data directory. The plan is displayed before
  anything happens; approval is a single confirmation; execution
  revalidates containment on every path, refuses to follow symlinks and
  junctions, records per-target failures, and verifies its results. An
  incomplete cleanup blocks the reseal.
- Concurrency protection for the handover through a machine-wide named
  mutex, and interrupt suppression while the approved sequence runs, so a
  mid-cleanup Ctrl+C cannot masquerade as a clean cancellation.
- The `winprep clean --dry-run` output now previews the cleanup plan, not
  just the preflight verdict.

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

[Unreleased]: https://github.com/developerasaad/WinPrep/compare/v0.2.0-alpha.1...HEAD
[0.2.0-alpha.1]: https://github.com/developerasaad/WinPrep/compare/v0.1.0-alpha.1...v0.2.0-alpha.1
[0.1.0-alpha.1]: https://github.com/developerasaad/WinPrep/releases/tag/v0.1.0-alpha.1
