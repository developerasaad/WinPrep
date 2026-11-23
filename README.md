# WinPrep

WinPrep is a portable Windows utility for laptop refurbishers and second-hand
computer sellers. It helps prepare a Windows installation for resale: verify
the machine is ready for repeated customer demonstrations, check its health
before a sale, and reseal Windows into first-time setup when the buyer takes
it.

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

## What it does

- **Prepare for display** (`winprep persist`) - verifies the machine is in a
  supported Audit Mode session and explains what that means for
  demonstrations. It makes no system changes: Audit Mode already survives
  restarts and shutdowns by design.
- **Check this laptop** (`winprep check`) - read-only diagnostics: computer
  details, storage devices with reported health, battery capacity and
  estimated health, Device Manager problem devices, and activation state.
  Reports can be exported; a Windows battery report can be generated.
- **Ready for buyer** (`winprep clean`) - the handover: a preflight safety
  inspection, removal of WinPrep's own recorded changes, a deliberate
  confirmation step, then `Sysprep.exe /oobe /shutdown` to reseal Windows
  into first-time setup. `winprep clean --dry-run` performs the full
  inspection without changing anything.
- **System information** (`winprep status`) - a read-only summary of the
  Windows installation, the current user and privileges, and the setup
  state, including Audit Mode detection.

## What it does not do

- It is **not a factory reset and not a secure erase**. Reseating with
  Sysprep leaves existing accounts, files, and traces on the disk. If the
  laptop's data must be gone, wipe the disk separately before selling it.
- It does not remove accounts or delete personal files. It points them out
  before a handover; removing them is the operator's decision.
- It does not install or update drivers, and it does not touch activation.
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
- An administrator console for `Ready for buyer` and for battery reports.
  Read-only commands work unelevated.
- Audit Mode for the full workflow (see [docs/workflow.md](docs/workflow.md)).
  WinPrep detects the machine's actual state and refuses state-changing
  operations outside the supported workflow.

## Getting started

1. Download `WinPrep-vX.Y.Z-windows-amd64.zip` from
   [Releases](https://github.com/developerasaad/WinPrep/releases) and verify
   it against `SHA256SUMS.txt`.
2. Extract `winprep.exe` anywhere, for example `C:\Tools`.
3. Open Command Prompt (as administrator for the full workflow) and run:

   ```
   C:\Tools\winprep.exe
   ```

The menu opens immediately:

```
WinPrep                                         v0.1.0
Windows laptop preparation

System:  Windows 11 Pro
Mode:    Audit Mode confirmed

  > Prepare for display
    Check this laptop
    Ready for buyer
    System information
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
winprep status             Read-only system summary
winprep persist            Verify the machine is ready for demonstrations
winprep check              Read-only diagnostics
winprep check --export C:\Users\Public\report.txt
winprep check --battery-report [path]
winprep clean              Prepare the laptop for handover (reseal into OOBE)
winprep clean --dry-run    Inspect only; report whether a handover is possible
winprep help               Full help
winprep version            Version and build target
```

Exit codes: `0` success, `1` failure or declined operation, `2` invalid
usage. `--yes` skips the interactive prompt on `clean`; the safety checks
still apply.

## The shop workflow

1. Install Windows on the laptop. When setup reaches the first region
   screen, press **Ctrl+Shift+F3**: the machine restarts into Audit Mode,
   signed in as the built-in Administrator.
2. Install drivers, apply updates, configure the machine for demonstration.
3. Run WinPrep and choose **Prepare for display**. It verifies the machine
   is in a supported Audit Mode session. Nothing else is needed - the laptop
   stays demonstrable across restarts and shutdowns.
4. Let customers test the laptop freely.
5. Before the sale, run **Check this laptop** and review the report.
6. Remove anything you created that should not reach the buyer (accounts,
   files, tools). WinPrep lists what it finds but deletes nothing of yours.
7. Run **Ready for buyer**, review the inspection, confirm the warnings,
   and let Sysprep reseal and shut down the laptop.
8. Switch the laptop on once yourself and confirm that Windows first-time
   setup appears. Only then hand it to the buyer.

Details and background: [docs/workflow.md](docs/workflow.md).

## Security limitations you should know

- **Audit Mode is an administrator session.** Anything a customer runs on a
  prepared laptop runs with full administrator rights. Audit Mode is a
  preparation environment, not a secure public kiosk.
- **Sysprep does not erase data.** A resealed laptop still contains
  everything from preparation and testing unless you wiped it.
- **The buyer's first boot must be verified by a human.** WinPrep reports
  that Sysprep ran; it cannot prove the resulting first-time setup works.
  Boot the machine yourself before selling it.

See [docs/safety.md](docs/safety.md) for the full discussion.

## Troubleshooting

Common problems - handover blocked by pending restarts, Sysprep failing on
provisioned Store apps, missing battery data - are covered in
[docs/troubleshooting.md](docs/troubleshooting.md), along with the Sysprep
log locations under `%WINDIR%\System32\Sysprep\Panther`.

## Contributing

Bug reports and pull requests are welcome. Read
[CONTRIBUTING.md](CONTRIBUTING.md) first; the safety rules in
[AGENTS.md](AGENTS.md) apply to human contributors as much as to tooling.

## License

[MIT](LICENSE)
