# WinPrep

WinPrep is a portable Windows utility for laptop refurbishers and second-hand
computer sellers. It does one job: keep a freshly installed Windows machine
usable in Audit Mode for repeated customer demonstrations, then clean the
demonstration data and hand the machine over through Windows' own
first-time-setup flow when it sells.

It is a single `winprep.exe` with no installation, no runtime dependencies,
and no network access. Run it from Command Prompt, or double-click it and
work in the console window it opens.

## The problem it solves

A refurbished laptop usually needs days or weeks on a shop shelf, and
customers want to try it before buying. Reinstalling Windows after every
test is impractical, and leaving a finished setup on the machine means the
buyer never gets the first-boot experience. Windows' own answer to this is
Audit Mode: the laptop stays in an unfinished, demonstrable state across
restarts, and a Sysprep call returns it to first-time setup when it sells.
WinPrep wraps that workflow in a tool a non-specialist can operate safely,
with the checks and warnings that prevent costly mistakes.

## The two operations

- **Prepare for Display** (`winprep persist`) - verifies the machine is in a
  supported Audit Mode session and explains what that means for
  demonstrations. It makes no system changes: Audit Mode already survives
  restarts and shutdowns by design, so there is nothing to configure and the
  step is safe to repeat.
- **Clean & Finish** (`winprep clean`) - the handover. WinPrep inspects the
  machine, builds an explicit cleanup plan from what customers actually used
  (the Downloads contents of the demonstration account, its temporary files,
  the browsing data of Edge and Chrome), shows the plan, and asks for one
  deliberate confirmation. Only then does it remove the approved data and
  run `Sysprep.exe /oobe /shutdown`, which reseals Windows into first-time
  setup and shuts the laptop down. `winprep clean --dry-run` shows the
  inspection and the plan without changing anything.

## What it does not do

- It is **not a factory reset and not a secure erase**. Resealing with
  Sysprep keeps the Windows installation, its drivers, and its programs, and
  removes nothing outside the approved cleanup scope. If the laptop's disk
  must be wiped, do that separately before selling it.
- It never deletes user accounts, and it never touches Desktop, Documents,
  Pictures, or other personal folders - it reports them so the operator can
  review what the buyer would receive.
- It does not install or update drivers, and it does not manage activation.
- It never disables Windows security features (Defender, firewall, UAC,
  BitLocker, SmartScreen).
- It contains no telemetry, no update checks, and no advertising. It works
  offline.

## Supported systems

Windows 10 and Windows 11, x64. Note that Windows 10 passed its general
support end date; WinPrep still runs there, but Microsoft no longer provides
general security updates for it.

## Requirements

- A Windows 10/11 x64 machine.
- An administrator console for Clean & Finish. Read-only commands work
  unelevated.
- Audit Mode for the full workflow (see [docs/workflow.md](docs/workflow.md)).
  WinPrep detects the machine's actual state and refuses state-changing
  operations outside the supported workflow.

## Getting started

1. Download `WinPrep-vX.Y.Z-windows-amd64.zip` from
   [Releases](https://github.com/developerasaad/WinPrep/releases) and verify
   it against `SHA256SUMS.txt`.
2. Extract `winprep.exe` to a USB stick or a folder that is not part of the
   demonstration data, for example `C:\Tools`. (Running it from the
   demonstration account's Downloads folder is refused by the cleanup plan,
   because deleting the running program fails midway.)
3. Open Command Prompt as administrator and run:

   ```
   C:\Tools\winprep.exe
   ```

The menu opens immediately:

```
WinPrep                                         v0.2.0
Windows preparation

Current mode: Audit Mode confirmed
Status: Ready for display

  > Prepare for Display
    Clean & Finish
    Help
    Exit

----------------------------------------------------------------------
UP/DOWN Move   ENTER Select   CTRL+C Exit
```

Controls: arrow keys move, ENTER selects, ESC backs out and cancels running
checks, Y/N answers warnings, CTRL+C exits. Everything is reachable with the
keyboard alone.

If the console cannot host the full-screen interface, WinPrep falls back to
a plain numbered menu. Without a terminal at all it prints guidance and the
equivalent commands.

## Command-line usage

```
winprep                    Interactive menu
winprep persist            Verify the machine is ready for demonstrations
winprep clean              Clean the approved data and prepare for handover
winprep clean --dry-run    Inspect and preview the cleanup; change nothing
winprep help               Full help
winprep version            Version and build target
```

Exit codes: `0` success, `1` failure or declined operation, `2` invalid
usage. `--yes` skips the interactive prompt on `clean`; the safety checks
still apply and the plan is still shown in full.

## The shop workflow

1. Install Windows on the laptop. When setup reaches the first region
   screen, press **Ctrl+Shift+F3**: the machine restarts into Audit Mode,
   signed in as the built-in Administrator.
2. Install drivers, apply updates, configure the machine for demonstration.
3. Run WinPrep and choose **Prepare for Display**. It verifies the machine
   is in a supported Audit Mode session. Nothing else is needed - the laptop
   stays demonstrable across restarts and shutdowns.
4. Let customers test the laptop freely.
5. Remove any accounts you created before the sale; WinPrep blocks the
   handover until only the built-in Administrator remains, and it never
   deletes accounts itself.
6. Run **Clean & Finish**. Review the inspection and the cleanup plan,
   confirm once, and let WinPrep remove the approved demonstration data and
   reseal Windows. The laptop shuts down.
7. Switch the laptop on once yourself and confirm that Windows first-time
   setup appears. Only then hand it to the buyer.

Details and background: [docs/workflow.md](docs/workflow.md).

## Security limitations you should know

- **Audit Mode is an administrator session.** Anything a customer runs on a
  prepared laptop runs with full administrator rights. Audit Mode is a
  preparation environment, not a secure public kiosk.
- **The cleanup is thorough but not a forensic wipe.** Deleted files can
  remain recoverable until the disk reuses the space. If the laptop's data
  must be unrecoverable, wipe the disk separately.
- **The buyer's first boot must be verified by a human.** WinPrep reports
  that Sysprep ran; it cannot prove the resulting first-time setup works.
  Boot the machine yourself before selling it.

See [docs/safety.md](docs/safety.md) for the full discussion.

## Troubleshooting

Common problems - handovers blocked by pending restarts, extra accounts, a
running browser, Sysprep failing on provisioned Store apps - are covered in
[docs/troubleshooting.md](docs/troubleshooting.md), along with the Sysprep
log locations under `%WINDIR%\System32\Sysprep\Panther`.

## Contributing

Bug reports and pull requests are welcome. Read
[CONTRIBUTING.md](CONTRIBUTING.md) first; the safety rules in
[AGENTS.md](AGENTS.md) apply to human contributors as much as to tooling.

## License

[MIT](LICENSE)
