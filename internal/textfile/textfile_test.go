package textfile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteUsesCRLF(t *testing.T) {
	path := filepath.Join(t.TempDir(), "report.txt")
	if err := Write(path, "line one\nline two\n"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "line one\r\nline two\r\n") {
		t.Errorf("content = %q, want CRLF line endings", data)
	}
}

func TestWriteNormalizesExistingCRLF(t *testing.T) {
	path := filepath.Join(t.TempDir(), "report.txt")
	if err := Write(path, "a\r\nb\n"); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	if strings.Contains(string(data), "\r\r") {
		t.Errorf("content = %q, double carriage returns", data)
	}
}

func TestWriteRejectsEmptyPath(t *testing.T) {
	if err := Write("", "x"); err == nil {
		t.Error("an empty path must be rejected")
	}
}

func TestWriteRejectsDirectory(t *testing.T) {
	dir := t.TempDir()
	if err := Write(dir, "x"); err == nil {
		t.Error("a directory path must be rejected")
	}
}
