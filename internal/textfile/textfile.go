// Package textfile writes plain-text report files for the CLI and TUI. Files
// are UTF-8 with Windows line endings so they open cleanly in Notepad on
// every supported Windows release.
package textfile

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Write saves content to path as UTF-8 text with CRLF line endings. An
// existing directory at path is refused rather than overwritten.
func Write(path, content string) error {
	if path == "" {
		return fmt.Errorf("no file path given")
	}
	if info, err := os.Stat(path); err == nil && info.IsDir() {
		return fmt.Errorf("%s is a directory", path)
	}
	content = strings.ReplaceAll(content, "\r\n", "\n")
	content = strings.ReplaceAll(content, "\n", "\r\n")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", filepath.Clean(path), err)
	}
	return nil
}
