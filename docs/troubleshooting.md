# Troubleshooting

Known failure modes, what WinPrep shows, and the verified next steps. For
Sysprep problems, the logs under `%WINDIR%\System32\Sysprep\Panther`
(`setupact.log`, `setuperr.log`) and `%WINDIR%\Panther` are the primary
evidence; WinPrep points at them whenever a handover fails.

## The handover is refused

The refusal message names the blocking checks. Common causes:

- **"Not running as administrator"** - Close the window, start Command
  Prompt with "Run as administrator", and try again.
- **"Windows has a restart pending"** - Component servicing or Windows
  Update is waiting for a reboot. Restart, let updates finish installing,
  and run **Ready for buyer** again. Sysprep is likely to fail while
  servicing operations are pending, so this is a hard block.
- **"This Windows installation is already deployed"** - The machine is in
  normal use, not Audit Mode. WinPrep will not reseal a deployed
  installation; the supported workflow starts with Audit Mode entered
  during Windows setup (see [workflow.md](workflow.md)). Re-running Sysprep
  on a machine that has been in normal use is not the supported path and
  can fail in ways that are hard to recover from.
- **"The Windows setup state could not be determined"** - The registry
  signals were missing or conflicting. Restart and try again; if it
  persists, please file a bug report with the output of `winprep status`.
- **"A Sysprep process is already running"** - Finish or close the existing
  Sysprep window, then retry.

## Sysprep itself fails

When Sysprep starts but reports an error, WinPrep shows the exit code and
the log locations. The most common cause on refurbished machines:

- **Provisioned Windows Store apps** - Sysprep can fail when a provisioned
  app installed during preparation cannot be removed for the new user.
  Microsoft documents the diagnosis and the supported removal procedure in
  ["Sysprep fails after you remove or update Windows Store apps"](https://learn.microsoft.com/en-us/troubleshoot/windows-client/setup-upgrade-and-drivers/sysprep-fails-remove-or-update-store-apps).
  The short version: remove the offending provisioned app package with
  `Remove-AppxProvisionedPackage`, restart, and retry the handover. Do not
  remove app packages wholesale as a precaution; remove the one named in
  `setupact.log`.

Other things worth checking when Sysprep fails:

- Pending restarts (see above) - restart first.
- Corrupt component store - `DISM /Online /Cleanup-Image /RestoreHealth`
  from an elevated prompt, restart, retry.
- Third-party antivirus or disk encryption agents - these are known to
  interfere with Sysprep; disable or uninstall them on the machine being
  prepared (this is not WinPrep disabling security; it is you deciding what
  runs on a laptop you are preparing for sale).

## The laptop boots to a "Sysprep is validating your installation" loop

This happens when Audit Mode was left in an inconsistent state - most
often when a reseal was interrupted. The machine is caught mid-transition.
Recovery usually means reinstalling Windows on the machine. This is why
WinPrep blocks a second Sysprep while one is running, refuses to run
Sysprep during pending servicing, and never kills a running Sysprep
process. If it happens anyway, the Panther logs tell you which phase
failed.

## Battery information is missing

`winprep check` explains per-field why a value is unavailable instead of
guessing:

- **"No battery was detected"** - The machine may not have a battery, or
  Windows cannot see it (a disconnected or failed battery often reports
  this way on laptops assembled for parts).
- **"did not report its design capacity"** - The battery firmware does not
  expose the data Windows needs. Health cannot be calculated, and WinPrep
  will not display a made-up percentage.
- The battery report (`winprep check --battery-report`, elevated) generates
  Windows' own HTML report with the raw data.

A battery that reports data can still be near failure; the health
percentage is an estimate from the battery's own counters, not a guarantee.

## Storage health is missing or "media type not reported"

Some storage drivers do not implement the queries Windows uses
(`Get-PhysicalDisk`). WinPrep falls back to a basic disk listing and marks
the health line unavailable rather than showing nothing. A drive that
reports "Healthy" is reporting its own opinion; a drive that reports
"Warning" or "Unhealthy" deserves a closer look with the manufacturer's
tool before the sale.

## The interactive menu does not start

- Running from a script or a service with no console: WinPrep prints
  guidance and the equivalent commands instead of a broken menu. This is
  intended; use the CLI commands.
- The console opens but the full-screen interface fails (rare - restricted
  terminal emulation): WinPrep falls back to a plain numbered menu.
- Garbled characters in a very old console font: WinPrep renders ASCII
  only; switch the console font to Consolas or another TrueType font if
  spacing looks wrong.

## winprep check shows "Some information was unavailable"

This is the honest outcome when Windows cannot answer a query - most often
the hardware query on locked-down systems. The notes next to each field say
which. It is not evidence of a fault, and a fully answered report is not
proof of health. The check complements a physical inspection; it does not
replace one.

## Something else

Open an issue with the output of `winprep status` and, for handover
problems, the relevant lines from the Sysprep logs. The issue template
asks for exactly that.
