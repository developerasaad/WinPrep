# The WinPrep workflow

This document explains the real-world workflow WinPrep supports: how a
refurbished laptop goes from a fresh Windows installation to a buyer's
first boot. It is written for shop operators; the technical background is
kept to what the decisions require.

## Why Audit Mode

Windows offers two ways to leave the out-of-box experience (OOBE): finish
setup normally, or press **Ctrl+Shift+F3** to enter Audit Mode before setup
completes. Audit Mode is Windows' own preparation state:

- The machine signs in automatically as the built-in Administrator.
- OOBE is skipped; the desktop appears directly.
- The state survives restarts and shutdowns. A laptop in Audit Mode stays
  in Audit Mode until something ends it.
- The intended exit is Sysprep with `/oobe`, which reseals the installation
  so the next boot presents first-time setup again.

That is exactly the lifecycle a refurbisher needs: a machine that stays
demonstrable for weeks and can be handed over in a "like new" state without
reinstalling Windows.

Microsoft's documentation covers the mechanics:
[Boot Windows to Audit Mode or OOBE](https://learn.microsoft.com/en-us/windows-hardware/manufacture/desktop/boot-windows-to-audit-mode-or-oobe)
and [Sysprep command-line options](https://learn.microsoft.com/en-us/windows-hardware/manufacture/desktop/sysprep-command-line-options).

## Phase 1: preparing the laptop

1. Install Windows 10 or 11. During OOBE - the first screens asking for
   region and keyboard - press **Ctrl+Shift+F3**.
2. The machine restarts into Audit Mode. The System Preparation Tool
   (Sysprep) window appears; you can close it with its close button.
   Closing the window does not end Audit Mode.
3. Install drivers, run updates, configure the software you demonstrate.
   Sign-in is automatic; no accounts are created.
4. Run WinPrep and choose **Prepare for display**.

"Prepare for display" verifies the machine is in a supported Audit Mode
session and reports anything that needs attention. It deliberately makes no
system changes. Tools that "keep Audit Mode alive" with scheduled tasks or
startup scripts are solving a problem Windows has already solved, and a
startup task that kills `sysprep.exe` can break a later reseal. WinPrep
does not do this.

## Phase 2: the shelf

A laptop in Audit Mode can be restarted, shut down, and used by customers
without leaving Audit Mode. Two things are worth knowing:

- Everything runs as the built-in Administrator. Audit Mode is a
  preparation environment, not a secure kiosk - see
  [safety.md](safety.md).
- Anything installed or saved during this phase stays on the machine until
  someone removes it. Sysprep will not remove it either.

## Phase 3: the sale

1. Remove any accounts you created while preparing the laptop (Settings >
   Accounts). Accounts survive the reseal, so WinPrep refuses the handover
   until only the built-in Administrator remains - and it never deletes
   accounts itself. Let Windows remove the account's files when it offers.
2. Close any open browsers and the System Preparation Tool window if it is
   on screen. Browsing data cannot be cleaned while its browser runs, and
   WinPrep never launches Sysprep while another Sysprep process exists.
3. Run **Clean & Finish**. WinPrep inspects the machine and builds the
   cleanup plan from the demonstration profile - the built-in
   Administrator's Downloads contents, the profile's temporary files, and
   the browsing data of Edge and Chrome - and shows everything:
   - **Ready** - all checks passed, the plan lists what will be removed.
   - **Requires attention** - for example, the idle System Preparation Tool
     window is open, or file renames are pending. You can proceed after
     acknowledging the items.
   - **Blocked** - for example, Windows has a restart pending, extra
     accounts or profiles remain, a browser is still running, or the
     machine is not in a confirmed Audit Mode session. The handover is
     refused until the issue is resolved. There is no override flag.
4. Review the plan. Folders the cleanup will not touch (Desktop, Documents,
   Pictures, and similar) are listed for your review; if a customer left
   something there that the buyer should not see, remove it yourself.
5. Confirm once. WinPrep re-checks the machine, removes exactly the approved
   data, verifies the results, and runs
   `Sysprep.exe /oobe /shutdown`.
   The laptop reseals and shuts down. If any removal fails, the reseal does
   not happen; close whatever holds the files and run Clean & Finish again -
   it safely retries what remains.
6. **Switch the laptop on once yourself.** Confirm that Windows first-time
   setup appears and works up to the point of account creation. WinPrep
   reports that Sysprep ran and exited cleanly; only a human can confirm
   the buyer's actual first boot experience.

A dry run - `winprep clean --dry-run` - shows the inspection and the plan
without changing anything. It is the safe way to see what a handover would
do on a given machine.

## Why no /generalize

Sysprep's `/generalize` switch removes machine-specific information to
prepare an image for deployment to *other* machines. A laptop being sold is
handed over on the same hardware, so `/generalize` is neither required nor
useful here, and it adds failure modes (driver re-detection, activation
changes) without benefit. WinPrep runs `/oobe /shutdown` only.

## What the handover is not

- **Not a factory reset.** Applications, drivers, and settings installed
  during preparation remain.
- **Not a secure erase.** Files created during preparation and customer
  testing remain on the disk and can be recovered with ordinary tools.
- **Not proof of a correct first boot.** Verify it yourself.

If a laptop must leave the shop with no recoverable data, wipe the disk
(microsoft's `cipher /w`, a manufacturer diagnostic tool, or a dedicated
wiping tool) before the final Windows installation, and prepare that
installation in Audit Mode as usual.
