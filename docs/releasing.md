# Releasing

The release process is automated by `.github/workflows/release.yml`; this
document describes what it does, what is expected of the maintainer, and
the manual checks that remain.

## Preconditions

- `main` is green in CI (both jobs).
- `internal/app/version.go` carries the version being released.
- `CHANGELOG.md` has an entry for the version.
- Release notes exist at `.github/release-notes/vX.Y.Z.md`.
- If the full Windows lifecycle test (see docs/development.md) has not been
  executed for this version, the version carries a prerelease label such as
  `-alpha.1` and the notes say plainly what has not been verified. Do not
  publish an unverified build as a stable release.

## Procedure

1. Merge or commit the version bump, changelog, and release notes.
2. Tag and push:

   ```
   git tag vX.Y.Z
   git push origin vX.Y.Z
   ```

3. The workflow then, in order:
   - validates the tag against the semantic-version pattern;
   - verifies the tag matches the `Version` constant (a mismatch fails the
     release rather than shipping a mislabeled binary);
   - runs gofmt, `go vet`, and the unit test suite;
   - builds `winprep.exe` for windows/amd64 with `-trimpath`, stripping
     (`-s -w`), and the commit hash injected into the version banner;
   - packages `WinPrep-vX.Y.Z-windows-amd64.zip` containing `winprep.exe`,
     `README.md`, and `LICENSE`;
   - generates `SHA256SUMS.txt` from the actual archive;
   - attests the archive's build provenance (GitHub artifact attestations);
   - creates the GitHub release with `gh release create`, marking versions
     containing `-` as prereleases;
   - uploads the archive as a workflow artifact for inspection.

4. After the workflow completes, verify by hand:
   - the release page shows the zip and the checksums;
   - `sha256sum` of the downloaded zip matches `SHA256SUMS.txt`;
   - the zip contains `winprep.exe`, `README.md`, `LICENSE` and nothing
     else;
   - the exe's `version` output reports the tag and commit;
   - the release is marked as a prerelease if, and only if, the version
     carries a prerelease label.

## Manual alternative

If the workflow cannot run (for example a runner incident), the same steps
work locally from a clean checkout:

```
GOOS=windows GOARCH=amd64 go build -trimpath -ldflags "-s -w -X github.com/developerasaad/winprep/internal/app.BuildCommit=$(git rev-parse --short HEAD)" -o dist/winprep.exe ./cmd/winprep
cd dist && zip WinPrep-vX.Y.Z-windows-amd64.zip winprep.exe LICENSE README.md && sha256sum *.zip > SHA256SUMS.txt
gh release create vX.Y.Z --verify-tag --notes-file ... dist/WinPrep-vX.Y.Z-windows-amd64.zip dist/SHA256SUMS.txt
```

Prefer the workflow: it is the reproducible path, and it performs the
tag/version checks a manual pass can forget.
