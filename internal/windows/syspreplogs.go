//go:build windows

package windows

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// SysprepMonitor implements platform.SysprepMonitor against the real Sysprep
// log locations.
type SysprepMonitor struct{}

// NewSysprepMonitor returns the Windows Sysprep monitor.
func NewSysprepMonitor() *SysprepMonitor { return &SysprepMonitor{} }

// SysprepLogTimes returns the modification times of the Sysprep Panther logs
// that exist. Missing logs mean Sysprep has not logged anything (yet), which
// is itself evidence of an idle window rather than an active operation.
func (m *SysprepMonitor) SysprepLogTimes(ctx context.Context) ([]time.Time, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	windir := os.Getenv("WINDIR")
	if windir == "" {
		return nil, errors.New("WINDIR is not set")
	}
	var times []time.Time
	for _, name := range []string{"setupact.log", "setuperr.log"} {
		p := filepath.Join(windir, "System32", "Sysprep", "Panther", name)
		info, err := os.Stat(p)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, fmt.Errorf("reading %s: %w", p, err)
		}
		times = append(times, info.ModTime())
	}
	return times, nil
}
