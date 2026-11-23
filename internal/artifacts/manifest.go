// Package artifacts tracks the changes WinPrep itself makes to a system, so
// the "Ready for buyer" step can remove exactly those changes and nothing
// else. WinPrep operates on an explicit ownership model: only entries recorded
// in the manifest are eligible for removal, and removal refuses paths outside
// the WinPrep data directory as a defense against tampered manifests.
package artifacts

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// EntryType classifies a managed artifact. v0.1 only creates files and
// directories; the registry type exists so future versions can record
// configuration changes without a format break.
type EntryType string

const (
	TypeFile      EntryType = "file"
	TypeDirectory EntryType = "directory"
	TypeRegistry  EntryType = "registry"
)

// Entry is one WinPrep-created artifact.
type Entry struct {
	Type      EntryType `json:"type"`
	Path      string    `json:"path"`
	CreatedAt time.Time `json:"created_at"`
	Note      string    `json:"note,omitempty"`
}

// Manifest is the on-disk record of everything WinPrep has added to the
// system. SchemaVersion allows future format changes.
type Manifest struct {
	SchemaVersion int       `json:"schema_version"`
	UpdatedAt     time.Time `json:"updated_at"`
	Entries       []Entry   `json:"entries"`
}

// SchemaVersionV1 is the current manifest format.
const SchemaVersionV1 = 1

// ErrOutsideDataDir is returned when an operation would touch a path outside
// the WinPrep data directory. It protects against manipulated manifests
// pointing WinPrep at unrelated files.
var ErrOutsideDataDir = errors.New("path is outside the WinPrep data directory")

// Store loads and persists the manifest at a fixed location.
type Store struct {
	// Dir is the WinPrep data directory (typically %ProgramData%\WinPrep).
	// All file entries must live inside it.
	Dir string
}

// NewStore returns a store rooted at dir.
func NewStore(dir string) *Store { return &Store{Dir: dir} }

// Path returns the manifest file location.
func (s *Store) Path() string { return filepath.Join(s.Dir, "managed-artifacts.json") }

// Load reads the manifest. A missing file is an empty manifest, not an error.
// A corrupt file is an error; silently ignoring it could hide artifacts.
func (s *Store) Load() (Manifest, error) {
	data, err := os.ReadFile(s.Path())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Manifest{SchemaVersion: SchemaVersionV1}, nil
		}
		return Manifest{}, fmt.Errorf("reading manifest: %w", err)
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return Manifest{}, fmt.Errorf("parsing manifest %s: %w", s.Path(), err)
	}
	if m.SchemaVersion == 0 {
		m.SchemaVersion = SchemaVersionV1
	}
	if m.SchemaVersion > SchemaVersionV1 {
		return Manifest{}, fmt.Errorf("manifest %s was written by a newer WinPrep version (schema %d); upgrade WinPrep before continuing", s.Path(), m.SchemaVersion)
	}
	return m, nil
}

// Save writes the manifest atomically: content is staged in a temporary file
// in the same directory and renamed into place.
func (s *Store) Save(m Manifest) error {
	m.SchemaVersion = SchemaVersionV1
	m.UpdatedAt = time.Now()
	if err := os.MkdirAll(s.Dir, 0o700); err != nil {
		return fmt.Errorf("creating WinPrep data directory: %w", err)
	}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding manifest: %w", err)
	}
	tmp, err := os.CreateTemp(s.Dir, "managed-artifacts-*.tmp")
	if err != nil {
		return fmt.Errorf("staging manifest: %w", err)
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return fmt.Errorf("writing staged manifest: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("closing staged manifest: %w", err)
	}
	if err := os.Rename(tmpName, s.Path()); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("replacing manifest: %w", err)
	}
	return nil
}

// Add records a new entry. Adding an identical (type, path) pair twice is a
// no-op, which keeps WinPrep's operations idempotent.
func (m *Manifest) Add(e Entry) bool {
	for _, existing := range m.Entries {
		if existing.Type == e.Type && strings.EqualFold(existing.Path, e.Path) {
			return false
		}
	}
	if e.CreatedAt.IsZero() {
		e.CreatedAt = time.Now()
	}
	m.Entries = append(m.Entries, e)
	return true
}

// Contains reports whether an entry with the given type and path is recorded.
func (m *Manifest) Contains(t EntryType, path string) bool {
	for _, e := range m.Entries {
		if e.Type == t && strings.EqualFold(e.Path, path) {
			return true
		}
	}
	return false
}

// Empty reports whether any artifacts are recorded.
func (m *Manifest) Empty() bool { return len(m.Entries) == 0 }

// Remove deletes the manifest file. Removing a missing file is not an error;
// the goal state is "no manifest present".
func (s *Store) Remove() error {
	if err := os.Remove(s.Path()); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("removing manifest: %w", err)
	}
	return nil
}

// EnsureInsideDataDir verifies that path is a file-system path located within
// the store's data directory. It is the ownership guard applied before any
// managed file is created or deleted.
func (s *Store) EnsureInsideDataDir(path string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("resolving %s: %w", path, err)
	}
	root, err := filepath.Abs(s.Dir)
	if err != nil {
		return fmt.Errorf("resolving %s: %w", s.Dir, err)
	}
	rel, err := filepath.Rel(root, abs)
	if err != nil {
		return ErrOutsideDataDir
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("%w: %s", ErrOutsideDataDir, path)
	}
	return nil
}
