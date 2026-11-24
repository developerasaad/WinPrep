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
  and run **Clean & Finish** again. Sysprep is likely to fail while
  servicing operations are pending, so this is a hard block.
- **"This Windows installation is already deployed"** - The machine is in
  normal use, not Audit Mode. WinPrep will not reseal a deployed
  installation; the supported workflow starts with Audit Mode entered
  during Windows setup (see [workflow.md](workflow.md)). Re-running Sysprep
  on a machine that has been in normal use is not the supported path and
  can fail in ways that are hard to recover from.
- **"Additional local accounts exist"** - Accounts survive the reseal and
  would be handed to the buyer. Remove them in Settings > Accounts (or
  lusrmgr.msc), let Windows delete their data, and run **Clean & Finish**
  again. WinPrep never deletes accounts itself.
- **"Additional user data is present"** - Profile folders or orphaned
  profile directories remain. Remove them through Windows (System
  Properties > User Profiles) so Windows cleans them up, then retry.
- **"The Windows setup state could not be determined"** - The registry
  signals were missing or conflicting. Restart and try again; if it
  persists, please file a bug report with the inspection output.
- **"Microsoft Edge / Google Chrome is still running"** - Browsing data
  cannot be removed while its browser runs. Close it fully - note that
  Edge's "Startup Boost" feature keeps background processes alive after
  you close the window - and retry.
- **"A Sysprep operation appears to be in progress"** - Sysprep's logs were
  written seconds ago, so an operation is running. Wait for it to finish
  (the machine may shut down), then retry.

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

## The idle System Preparation Tool window

Audit Mode opens the Sysprep window at every sign-in. It can be closed with
its close button; that does not end Audit Mode. WinPrep reports it as an
attention item during the inspection and refuses to launch its own Sysprep
while the window's process exists - close it before the final step of
**Clean & Finish**. WinPrep never closes or kills it itself, because
interrupting an active Sysprep operation can leave Windows in a broken
state, and telling an idle window from a running operation by process name
alone is not reliable.

## The cleanup did not complete

When a removal fails - usually because a running program holds a file open,
or permissions changed - WinPrep reports the failing paths, does not start
the reseal, and tells you to retry. The cleanup is safe to re-run: it
removes what remains and treats already-absent files as done. Close the
program holding the files (the report usually names the browser or folder),
then run **Clean & Finish** again and re-confirm the new plan.

## The laptop boots to a "Sysprep is validating your installation" loop

This happens when Audit Mode was left in an inconsistent state - most
often when a reseal was interrupted. The machine is caught mid-transition.
Recovery usually means reinstalling Windows on the machine. This is why
WinPrep blocks a second Sysprep while one is running, refuses to run
Sysprep during pending servicing, and never kills a running Sysprep
process. If it happens anyway, the Panther logs tell you which phase
failed.

## The interactive menu does not start

- Running from a script or a service with no console: WinPrep prints
  guidance and the equivalent commands instead of a broken menu. This is
  intended; use the CLI commands.
- The console opens but the full-screen interface fails (rare - restricted
  terminal emulation): WinPrep falls back to a plain numbered menu.
- Garbled characters in a very old console font: WinPrep renders ASCII
  only; switch the console font to Consolas or another TrueType font if
  spacing looks wrong.

## Something else

Open an issue with the inspection output (`winprep clean --dry-run` is a
safe way to capture it) and, for handover problems, the relevant lines from
the Sysprep logs. The issue template asks for exactly that.
