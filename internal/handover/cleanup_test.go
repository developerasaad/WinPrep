package handover

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/developerasaad/winprep/internal/artifacts"
)

func newTestFlow(t *testing.T) (*Flow, *artifacts.Store) {
	t.Helper()
	dir := t.TempDir()
	store := artifacts.NewStore(dir)
	flow := &Flow{Store: store}
	return flow, store
}

func TestCleanupRemovesRecordedArtifacts(t *testing.T) {
	flow, store := newTestFlow(t)

	note := filepath.Join(store.Dir, "demo-note.txt")
	if err := os.WriteFile(note, []byte("managed by winprep"), 0o600); err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(store.Dir, "subdir")
	if err := os.MkdirAll(sub, 0o700); err != nil {
		t.Fatal(err)
	}

	m := artifacts.Manifest{}
	m.Add(artifacts.Entry{Type: artifacts.TypeFile, Path: note})
	m.Add(artifacts.Entry{Type: artifacts.TypeDirectory, Path: sub})
	if err := store.Save(m); err != nil {
		t.Fatal(err)
	}

	results, err := flow.CleanupArtifacts(t.Context())
	if err != nil {
		t.Fatalf("CleanupArtifacts() error = %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 cleanup results, got %d", len(results))
	}
	for _, r := range results {
		if !r.Removed || r.Err != "" {
			t.Errorf("entry %s not removed: %s", r.Entry.Path, r.Err)
		}
	}
	if _, err := os.Stat(note); !os.IsNotExist(err) {
		t.Error("managed file still exists after cleanup")
	}
	if _, err := os.Stat(sub); !os.IsNotExist(err) {
		t.Error("managed directory still exists after cleanup")
	}
	if _, err := os.Stat(store.Path()); !os.IsNotExist(err) {
		t.Error("manifest still exists after successful cleanup")
	}
}

func TestCleanupRefusesPathsOutsideDataDir(t *testing.T) {
	flow, store := newTestFlow(t)
	outside := filepath.Join(t.TempDir(), "innocent.txt")
	if err := os.WriteFile(outside, []byte("user data"), 0o600); err != nil {
		t.Fatal(err)
	}

	m := artifacts.Manifest{}
	m.Add(artifacts.Entry{Type: artifacts.TypeFile, Path: outside})
	if err := store.Save(m); err != nil {
		t.Fatal(err)
	}

	results, err := flow.CleanupArtifacts(t.Context())
	if err == nil {
		t.Error("cleanup with refused entries must report an error")
	}
	if len(results) != 1 || results[0].Removed {
		t.Fatalf("outside path must not be removed, got %+v", results)
	}
	if _, err := os.Stat(outside); err != nil {
		t.Error("file outside the data dir was deleted; the ownership guard failed")
	}
	if _, err := os.Stat(store.Path()); err != nil {
		t.Error("manifest must be kept when cleanup did not complete")
	}
}

func TestCleanupWithEmptyManifestRemovesManifestOnly(t *testing.T) {
	flow, store := newTestFlow(t)
	if err := store.Save(artifacts.Manifest{}); err != nil {
		t.Fatal(err)
	}
	results, err := flow.CleanupArtifacts(t.Context())
	if err != nil {
		t.Fatalf("CleanupArtifacts() error = %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected no per-entry results, got %d", len(results))
	}
	if _, err := os.Stat(store.Path()); !os.IsNotExist(err) {
		t.Error("empty manifest should still be removed")
	}
}

func TestCleanupMissingManifestIsFine(t *testing.T) {
	flow, _ := newTestFlow(t)
	if _, err := flow.CleanupArtifacts(t.Context()); err != nil {
		t.Fatalf("cleanup without a manifest should succeed, got %v", err)
	}
}

func TestCleanupRefusesRegistryEntriesFromNewerVersions(t *testing.T) {
	flow, store := newTestFlow(t)
	m := artifacts.Manifest{}
	m.Add(artifacts.Entry{Type: artifacts.TypeRegistry, Path: `HKLM\SOFTWARE\Example`})
	if err := store.Save(m); err != nil {
		t.Fatal(err)
	}
	results, err := flow.CleanupArtifacts(t.Context())
	if err == nil {
		t.Fatal("unsupported registry entry must cause an error")
	}
	if len(results) != 1 || results[0].Removed {
		t.Error("registry entry must not be touched")
	}
}
