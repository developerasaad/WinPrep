# Changelog

All notable changes to WinPrep are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and the project
uses semantic versioning.

## [Unreleased]

## [0.2.0-alpha.2] - 2026-11-24

Audit Mode detection did not work on real Windows, which blocked both
operations on every genuine Audit Mode machine. This release fixes the
detector and hardens the handover paths a review of the detection change
touched.

### Fixed

- Audit Mode detection now reads the setup state from the value real
  Windows installations write: `ImageState` under
  `HKLM\SOFTWARE\...\Setup\State`. The previous build queried `StateName`, a
  value name that appears in the prose of Microsoft's "Windows Setup States"
  page but has never been observed on a real installation, so every real
  machine evaluated to "setup state unknown".
- Detection no longer requires `HKLM\SYSTEM\Setup\CmdLine` to equal
  `audit.exe`. Field evidence (OEM deployment logs, support threads,
  production deployment tooling) shows that value holds whatever the setup
  engine queued before logon - `oobe\windeploy.exe`, `SetupComplete.cmd`,
  or nothing - never "audit.exe". It is now collected as a reported
  observation only. A confirmed Audit Mode session instead requires the
  setup state (`IMAGE_STATE_UNDEPLOYABLE` or a documented
  `IMAGE_STATE_*_RESEAL_TO_AUDIT` value), the audit boot flag
  (`HKLM\SYSTEM\Setup\Status\AuditBoot`, with the Windows 11-observed
  `AuditBootVolatile` sibling), no live OOBE, and the built-in
  Administrator session Microsoft documents for Audit Mode. The State.ini
  file and the documented `StateName` value are read as fallbacks; a
  disagreement between sources is reported as a conflict rather than
  guessed away.
- The handover revalidation fails closed when the running-process list
  cannot be read, instead of proceeding as if no Sysprep process existed.
- The full preflight evaluation is re-run after the operator confirms, so a
  blocker that appeared in the meantime (a new local account, a pending
  servicing restart, an active Sysprep operation) stops the handover before
  anything is deleted.
- The executor re-checks every approved directory root with `Lstat` right
  before enumerating it and refuses to traverse a root that became a
  symlink, junction, or other reparse point after planning. Previously only
  the final path components were checked, so a root swapped for a junction
  mid-run could redirect deletions outside the approved scope.
- A cleanup target directory that vanished between planning and verification
  no longer marks the run incomplete; the goal state (nothing left in it) is
  already met.
- A failed Sysprep path check (unset `WINDIR`, access denied) is now reported
  as "could not verify" instead of a false "Sysprep.exe is not present".
- Interrupt shielding during the approved handover no longer destroys other
  components' interrupt handlers (the TUI's) when it ends.
- The TUI status line no longer stays empty after the startup check is
  cancelled, stale notices no longer survive into the result and help
  screens, and Ctrl+C at the full-screen interface exits instead of dropping
  into the fallback menu.
- The `--dry-run` claim "no changes were made" is now literally true:
  running WinPrep no longer creates `%ProgramData%\WinPrep` on inspection
  alone (the directory is created when something is actually written).

### Changed

- The operation lock is released when a handover attempt ends without an
  initiated reseal, so a failed cleanup can be retried from the same window
  without restarting WinPrep. After an initiated reseal the lock stays held:
  the machine is shutting down.
- The cleanup executes the revalidated plan over the same approved
  locations, so demonstration data that appeared inside an approved location
  after the operator confirmed is covered by that confirmation instead of
  being silently left behind.
- The TUI confirmation screen now shows the inspection and cleanup plan
  above the warning (the alt screen has no scrollback), and the plan can be
  scrolled there with UP/DOWN.

### Added

- `winprep signals`, a read-only troubleshooting command that prints every
  setup-state signal the detector collected, where each value came from, and
  how the deployment mode was evaluated. It is the evidence to attach when
  a genuine Audit Mode session is rejected; docs/troubleshooting.md explains
  the procedure.
- Windows integration tests assert that `ImageState` is present on a real
  installation - the regression guard for the value-name defect - and the
  smoke tests skip themselves on a machine that reports a confirmed Audit
  Mode session rather than risk a real `clean --yes` handover.

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

[Unreleased]: https://github.com/developerasaad/WinPrep/compare/v0.2.0-alpha.2...HEAD
[0.2.0-alpha.2]: https://github.com/developerasaad/WinPrep/compare/v0.2.0-alpha.1...v0.2.0-alpha.2
[0.2.0-alpha.1]: https://github.com/developerasaad/WinPrep/compare/v0.1.0-alpha.1...v0.2.0-alpha.1
[0.1.0-alpha.1]: https://github.com/developerasaad/WinPrep/releases/tag/v0.1.0-alpha.1
