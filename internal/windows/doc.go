// Package windows implements the platform interfaces against the real Windows
// APIs: the registry, process and security token functions, and the documented
// Windows utilities (Sysprep, powercfg, Windows PowerShell). All files except
// this one carry a "windows" build constraint, which lets the rest of the
// module build and unit-test on any OS while this package type-checks under
// GOOS=windows.
package windows

// The implementation is split by concern:
//
//   - registry.go: read-only registry access (platform.Registry)
//   - system.go:   system and user facts from the registry and native calls
//   - procs.go:    process enumeration via the tool help snapshot
//   - runner.go:   external program execution (platform.Runner)
//   - paths.go:    well-known executable and data paths
