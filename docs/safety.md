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

## What the handover removes - and what it does not

Clean & Finish removes the approved demonstration data before resealing:
the Downloads contents and temporary files of the demonstration profile,
the browsing data (caches, history, cookies, saved logins) of Edge and
Chrome, and WinPrep's own data directory. Everything is listed in the plan
before anything happens, and nothing outside the approved locations is
touched.

What the handover deliberately does not remove:

- Accounts. Extra local accounts survive the reseal, so WinPrep blocks the
  handover until the operator has removed them through Windows; it never
  deletes accounts or profile folders itself.
- Personal folders. Desktop, Documents, Pictures, and similar are reported
  for review, not deleted.
- Applications or drivers.
- Event logs, registry history, or pagefile contents.

The cleanup is thorough but not a forensic wipe: deleted files can remain
recoverable until the disk reuses the space. If the data must be
unrecoverable, use a real wipe *before* preparing the laptop: reinstall
Windows on a wiped disk, then enter Audit Mode. Wiping after resealing
destroys the prepared installation.

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
- **"Sysprep exited cleanly, so first boot works."** WinPrep reports that
  the reseal was initiated, not that the buyer's experience is verified.
  Boot the machine yourself.

## Privilege handling

- `winprep clean` requires an elevated console and says so, with
  instructions, when it is not.
- WinPrep never elevates itself, never asks for a UAC prompt on the user's
  behalf, and never schedules tasks to run privileged later.
- Read-only commands (`persist`, `clean --dry-run`) work unelevated on the
  machines they can inspect; the SAM account query behind the handover
  inspection needs administrative rights, which the supported workflow
  already requires.

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
