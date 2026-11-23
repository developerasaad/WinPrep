# Safety notes

WinPrep operates on machines that are about to be sold. This document
explains the security properties of the workflow, what WinPrep refuses to
do, and the assumptions that cause real-world losses when they turn out to
be wrong.

## Audit Mode is an administrator session

Audit Mode signs in the built-in Administrator account (RID 500) with a
full token. There is no UAC prompt, no per-user isolation, and no sandbox.
Consequences for a shop:

- Anything a customer runs on a prepared laptop - intentionally or by
  visiting a hostile web page - runs with administrator rights.
- Malware planted during a demonstration has full access to the machine.
- The password-free administrator session is available to anyone who can
  power the laptop on.

Audit Mode is a *preparation* environment, not a secure public kiosk. If
customers test laptops unsupervised, treat the machines as potentially
compromised and reseal them for sale as the final step, after removing
anything you would not ship. If a laptop must be demonstrable *and*
hardened at the same time, that is a different product; WinPrep does not
pretend otherwise.

## What resealing actually removes - and what it does not

`Sysprep.exe /oobe /shutdown` reseals Windows so the next boot presents
first-time setup. It does not:

- Delete files. Everything written during preparation and customer testing
  is still on the disk.
- Remove accounts. Extra local accounts survive; their profile folders
  survive even if the accounts are deleted.
- Uninstall applications or drivers.
- Clear browser profiles, event logs, registry history, or pagefile
  contents.

WinPrep surfaces extra accounts and profile folders during the handover
inspection because they are easy to miss, but it never deletes them.
Deciding what leaves the shop is the operator's job.

If the data itself must be gone, use a real wipe *before* preparing the
laptop: reinstall Windows on a wiped disk, then enter Audit Mode. Wiping
after resealing destroys the prepared installation.

## Dangerous assumptions this project refuses

- **"Sysprep is a factory reset."** It is not. Selling a resealed laptop as
  "data-free" without a separate wipe transfers your data to the buyer.
- **"A startup task that kills sysprep.exe keeps things tidy."** A task that
  kills Sysprep on every boot can also kill it *during* the actual reseal,
  leaving Windows in a half-prepared state. WinPrep never creates such a
  task; it explains the window instead.
- **"The built-in Administrator proves Audit Mode."** The account exists on
  every installation. WinPrep's detection requires multiple consistent
  signals (setup state, setup command line, session SID) before it calls a
  machine Audit Mode, and treats conflicts as "do not act".
- **"The check passed, so the disk is healthy."** A successful storage query
  is a successful query. Health values are the drive's own self-report.
  WinPrep reports them with that caveat and never converts missing data
  into good news.
- **"Sysprep exited cleanly, so first boot works."** WinPrep reports that
  the reseal was initiated, not that the buyer's experience is verified.
  Boot the machine yourself.

## Privilege handling

- `winprep clean` requires an elevated console and says so, with
  instructions, when it is not.
- WinPrep never elevates itself, never asks for a UAC prompt on the user's
  behalf, and never schedules tasks to run privileged later.
- Read-only commands (`status`, `persist`, `check`) work unelevated. The
  battery report needs elevation because `powercfg /batteryreport` does;
  the error says exactly that.

## What WinPrep will never do

The initial version deliberately excludes, and the project's scope excludes
without a strong reason to change:

- Windows reinstallation, partition deletion, formatting, or disk wiping
- Automatic removal of unrelated accounts
- Silent driver installation
- Activation bypasses or license modification
- Disabling Windows Defender, the firewall, UAC, BitLocker, or SmartScreen
- Telemetry, update checks, or any network communication
- Self-update through remote scripts (`irm | iex` and similar)

## Reporting a safety problem

See [SECURITY.md](SECURITY.md). Use GitHub's private vulnerability
reporting; do not open a public issue for anything exploitable.
