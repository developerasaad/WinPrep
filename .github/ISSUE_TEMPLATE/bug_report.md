---
name: Bug report
about: Something behaved incorrectly or misleadingly
labels: bug
---

**What happened**

A clear description of what WinPrep did, and what you expected instead.

**What you ran**

The exact command or menu option, for example `winprep clean --dry-run` or
"Clean & Finish". If the problem involves Sysprep, say whether it was a
dry run or a real handover.

**Machine state**

- Windows edition and build (Win + R, run `winver`):
- In Audit Mode, normal installation, or first-time setup:
- Elevated Command Prompt or not:

**Output**

Paste the relevant output between the backticks. If Sysprep failed, also
check the logs WinPrep points to (`%WINDIR%\System32\Sysprep\Panther\setuperr.log`).

```

```
