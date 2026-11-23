# Security policy

## Supported versions

WinPrep is a small utility with a single supported version: the latest
release. Security fixes are released as patch versions; prerelease builds
(alpha, beta) are for evaluation and receive fixes only through new
prereleases.

## Reporting a vulnerability

Use GitHub's **private vulnerability reporting** on this repository
(Report a vulnerability, under the Security tab). This is the supported and
monitored channel. Please do not open a public issue for anything that
could be exploited, and do not include secrets, tokens, or customer data in
reports.

Include what you can of:

- the WinPrep version (`winprep version`);
- the Windows edition and build;
- a description of the issue and its impact;
- reproduction steps;
- any proof-of-concept material.

You will get an acknowledgement that the report was received. Fixes for
accepted issues are released as soon as practical, with credit to the
reporter in the release notes unless you prefer otherwise.

## What counts as a security issue here

WinPrep runs with administrator privileges on machines being prepared for
sale, so its threat surface matters. Examples of reports we want:

- any path where WinPrep deletes, modifies, or executes something it should
  not, including through a tampered artifact manifest;
- command construction that could be influenced to run unintended programs;
- privilege-related behavior beyond the documented, user-initiated Sysprep
  call;
- misleading output that could cause an operator to believe an unsafe
  machine was prepared safely.

Not security issues: Sysprep's own limitations, data remaining after a
reseal (documented and intended - see docs/safety.md), or Windows behavior
outside WinPrep's control.

## Scope of this project's security posture

WinPrep works entirely offline, collects nothing, and phones nothing home.
It never disables Windows security features. Audit Mode itself is an
administrator session; that property is documented in docs/safety.md and is
not something WinPrep can change.
