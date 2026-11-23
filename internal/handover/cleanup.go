package handover

import (
	"context"
	"fmt"
	"os"

	"github.com/developerasaad/winprep/internal/artifacts"
)

// CleanupResult reports the removal of one managed artifact.
type CleanupResult struct {
	Entry   artifacts.Entry
	Removed bool
	Err     string
}

// CleanupArtifacts removes the artifacts WinPrep itself recorded, and nothing
// else. Every entry is re-validated against the data directory ownership rule
// before deletion; a tampered manifest entry is refused, not executed.
func (f *Flow) CleanupArtifacts(context.Context) ([]CleanupResult, error) {
	manifest, err := f.Store.Load()
	if err != nil {
		return nil, fmt.Errorf("loading the artifact manifest: %w", err)
	}
	if manifest.Empty() {
		return nil, f.Store.Remove()
	}

	results := make([]CleanupResult, 0, len(manifest.Entries))
	for _, entry := range manifest.Entries {
		res := CleanupResult{Entry: entry}
		if err := f.Store.EnsureInsideDataDir(entry.Path); err != nil {
			res.Err = err.Error()
			results = append(results, res)
			continue
		}
		switch entry.Type {
		case artifacts.TypeFile, artifacts.TypeDirectory:
			if err := os.RemoveAll(entry.Path); err != nil {
				res.Err = err.Error()
			} else {
				res.Removed = true
			}
		case artifacts.TypeRegistry:
			// v0.1 records no registry artifacts. A registry entry in a
			// manifest can only come from a newer version; refuse rather
			// than guess how to undo it.
			res.Err = "registry artifacts are not supported by this WinPrep version"
		default:
			res.Err = fmt.Sprintf("unknown artifact type %q", entry.Type)
		}
		results = append(results, res)
	}

	// The manifest is removed only if every entry was handled successfully.
	// Keeping it after a failure preserves the record of what remains.
	for _, r := range results {
		if r.Err != "" {
			return results, fmt.Errorf("not all WinPrep artifacts could be removed; the manifest has been kept")
		}
	}
	return results, f.Store.Remove()
}
