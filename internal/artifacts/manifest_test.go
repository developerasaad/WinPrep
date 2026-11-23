package artifacts

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	return NewStore(t.TempDir())
}

func TestLoadMissingManifestIsEmpty(t *testing.T) {
	s := newTestStore(t)
	m, err := s.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if !m.Empty() {
		t.Fatalf("Load() of missing manifest should be empty, got %d entries", len(m.Entries))
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	s := newTestStore(t)
	m := Manifest{}
	m.Add(Entry{Type: TypeFile, Path: filepath.Join(s.Dir, "note.txt"), Note: "test artifact"})
	if err := s.Save(m); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	got, err := s.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(got.Entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(got.Entries))
	}
	if got.Entries[0].Path != filepath.Join(s.Dir, "note.txt") {
		t.Errorf("entry path = %q", got.Entries[0].Path)
	}
	if got.Entries[0].CreatedAt.IsZero() {
		t.Error("Add() should stamp CreatedAt")
	}
}

func TestAddIsIdempotent(t *testing.T) {
	m := Manifest{}
	first := m.Add(Entry{Type: TypeFile, Path: `C:\ProgramData\WinPrep\a.txt`})
	second := m.Add(Entry{Type: TypeFile, Path: `C:\ProgramData\WinPrep\a.txt`})
	if !first {
		t.Error("first Add() should record the entry")
	}
	if second {
		t.Error("duplicate Add() should be a no-op")
	}
	if len(m.Entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(m.Entries))
	}
	// A different type for the same path is a distinct artifact.
	third := m.Add(Entry{Type: TypeDirectory, Path: `C:\ProgramData\WinPrep\a.txt`})
	if !third {
		t.Error("same path with a different type is a different entry")
	}
}

func TestLoadRejectsCorruptManifest(t *testing.T) {
	s := newTestStore(t)
	if err := os.MkdirAll(s.Dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(s.Path(), []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Load(); err == nil {
		t.Fatal("Load() should fail on a corrupt manifest instead of hiding artifacts")
	}
}

func TestLoadRejectsNewerSchema(t *testing.T) {
	s := newTestStore(t)
	if err := os.MkdirAll(s.Dir, 0o700); err != nil {
		t.Fatal(err)
	}
	body := `{"schema_version": 99, "entries": []}`
	if err := os.WriteFile(s.Path(), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := s.Load()
	if err == nil || !strings.Contains(err.Error(), "newer WinPrep") {
		t.Fatalf("Load() should reject schema 99, got %v", err)
	}
}

func TestEnsureInsideDataDir(t *testing.T) {
	s := newTestStore(t)
	inside := filepath.Join(s.Dir, "sub", "file.txt")
	if err := s.EnsureInsideDataDir(inside); err != nil {
		t.Errorf("path inside data dir rejected: %v", err)
	}
	if err := s.EnsureInsideDataDir(filepath.Join(s.Dir, "..", "escape.txt")); err == nil {
		t.Error("path escaping the data dir must be rejected")
	} else if !errors.Is(err, ErrOutsideDataDir) {
		t.Errorf("expected ErrOutsideDataDir, got %v", err)
	}
	if err := s.EnsureInsideDataDir(t.TempDir() + "/other.txt"); err == nil {
		t.Error("path in another directory must be rejected")
	}
}

func TestRemoveMissingManifestIsNotAnError(t *testing.T) {
	s := newTestStore(t)
	if err := s.Remove(); err != nil {
		t.Fatalf("Remove() of a missing manifest should succeed, got %v", err)
	}
}

func TestRemoveDeletesManifest(t *testing.T) {
	s := newTestStore(t)
	if err := s.Save(Manifest{}); err != nil {
		t.Fatal(err)
	}
	if err := s.Remove(); err != nil {
		t.Fatalf("Remove() error = %v", err)
	}
	if _, err := os.Stat(s.Path()); !errors.Is(err, os.ErrNotExist) {
		t.Error("manifest file still present after Remove()")
	}
}

func TestSaveIsAtomicAndLeavesNoTempFiles(t *testing.T) {
	s := newTestStore(t)
	if err := s.Save(Manifest{}); err != nil {
		t.Fatal(err)
	}
	if err := s.Save(Manifest{}); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(s.Dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "managed-artifacts.json" {
		names := make([]string, 0, len(entries))
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Errorf("expected only the manifest file, got %v", names)
	}
}
