# Troubleshooting

Known failure modes, what WinPrep shows, and the verified next steps. For
Sysprep problems, the logs under `%WINDIR%\System32\Sysprep\Panther`
(`setupact.log`, `setuperr.log`) and `%WINDIR%\Panther` are the primary
evidence; WinPrep points at them whenever a handover fails.

## WinPrep does not recognize a genuine Audit Mode session

This is the most important case to report, because a machine that is really
in Audit Mode but reports "uncertain" or "unknown" means a signal assumption
no longer holds. Do **not** fix it by editing the setup-state registry values
by hand - that only masks which signal is actually missing and can leave the
machine in a state no tool can reason about.

What to do, in order:

1. Check the obvious causes first, which the refusal text names:
   - WinPrep must run inside the Audit Mode session. Boot the machine, let
     it sign in as the built-in Administrator, then run WinPrep there.
   - The session must be the built-in Administrator (RID 500). Audit Mode
     signs in as that account; if you created another account and signed in
     as it, sign out and back in as the built-in Administrator.
2. Run `winprep signals` in the same window. It prints every raw value the
   detector read, where each one came from, and how the state was evaluated.
   It is read-only and collects setup-state values and the session SID only.
3. Compare the "Session user SID" line with the output of `whoami /user`
   run in the same console.
4. If the report shows a signal you expected to be set (usually the audit
   boot flag, `HKLM\SYSTEM\Setup\Status\AuditBoot`) but it reads "not
   present" on a machine that is demonstrably in Audit Mode, that is exactly
   the evidence a bug report needs. Open an issue with the full
   `winprep signals` output and the `whoami /user` output.

If you prefer to collect the evidence by hand instead, these read-only
commands show the same values:

```
reg query "HKLM\SOFTWARE\Microsoft\Windows\CurrentVersion\Setup\State"
reg query "HKLM\SYSTEM\Setup"
reg query "HKLM\SYSTEM\Setup\Status" /v AuditBoot
type "%WINDIR%\Setup\State\State.ini"
whoami /user
```

How WinPrep decides, so the output makes sense: the setup state comes from
`ImageState` (the value real Windows installations write; the StateName
value named in older Microsoft documentation prose is read as a fallback),
and a live Audit Mode session is confirmed only when the state is
`IMAGE_STATE_UNDEPLOYABLE` or one of the documented
`IMAGE_STATE_*_RESEAL_TO_AUDIT` values, the audit boot flag is set, OOBE is
not in progress, and the session runs as the built-in Administrator.
Anything missing or contradictory stays "uncertain", and uncertain machines
get no state-changing operations - that is deliberate. In particular,
`HKLM\SYSTEM\Setup\CmdLine` is reported but never used as a gate: on real
systems that value carries whatever the setup engine queued before logon
(`oobe\windeploy.exe`, `SetupComplete.cmd`, or nothing), not "audit.exe".

Known limitation, stated honestly: the audit boot flag is confirmed by field
evidence on Windows 7 through 10 audit machines, but Microsoft does not
officially document it, and it has not been possible to verify its behavior
on a real Windows 11 Audit Mode installation during development. A Windows
11 machine that is genuinely in Audit Mode but reports the flag absent would
be classified "uncertain" with the reason spelled out - not misdetected. If
that happens to you, the `winprep signals` report is precisely what the bug
report needs to extend the rules with real evidence.

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
- **"The Windows setup state could not be determined"** or **"Setup state
  uncertain"** - The signals were missing, unreadable, or conflicting. See
  [WinPrep does not recognize a genuine Audit Mode
  session](#winprep-does-not-recognize-a-genuine-audit-mode-session) above;
  the `winprep signals` report shows exactly what was read.
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
