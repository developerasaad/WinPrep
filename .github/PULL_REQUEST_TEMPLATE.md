**What this changes**

A short description of the change and why. If it closes an issue, write
"Closes #N".

**How it was tested**

Which tests cover the change, and anything you verified by hand. If the
change touches Windows-specific behavior, say how it was checked (unit tests
with fakes, the CI Windows runner, or a real machine).

**Checklist**

- [ ] The change does not add telemetry, network calls, or background services.
- [ ] The change does not weaken safety checks or hide failures.
- [ ] Documentation and help text still match the behavior.
- [ ] `gofmt -l .` is clean, `go vet ./...` passes, `go test ./...` passes.
