package diagnostics

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ErrNotElevated is returned when a battery report is requested without
// administrative privileges, which powercfg requires.
var ErrNotElevated = errors.New("generating a battery report requires administrative privileges; run WinPrep as administrator")

// DefaultBatteryReportName is the file name used when the caller does not
// choose one.
const DefaultBatteryReportName = "winprep-battery-report.html"

// BatteryReport generates a Windows battery report with powercfg. The report
// is only written when the operator explicitly asks for it, to the path they
// chose; WinPrep never leaves files behind on its own.
func (s *Service) BatteryReport(ctx context.Context, outPath string) (string, error) {
	user, err := s.Sys.UserInfo(ctx)
	if err != nil {
		return "", fmt.Errorf("checking privileges: %w", err)
	}
	if !user.IsElevated {
		return "", ErrNotElevated
	}
	if outPath == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return "", fmt.Errorf("determining the current directory: %w", err)
		}
		outPath = filepath.Join(cwd, DefaultBatteryReportName)
	}
	if dir := filepath.Dir(outPath); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return "", fmt.Errorf("preparing the report location: %w", err)
		}
	}

	res, err := s.Run.Run(ctx, s.PowerCfgPath, "/batteryreport", "/output", outPath)
	if err != nil {
		return "", fmt.Errorf("starting powercfg: %w", err)
	}
	if res.ExitCode != 0 {
		detail := strings.TrimSpace(res.Stdout)
		if detail == "" {
			detail = strings.TrimSpace(res.Stderr)
		}
		if detail != "" {
			return "", fmt.Errorf("powercfg exited with code %d: %s", res.ExitCode, detail)
		}
		return "", fmt.Errorf("powercfg exited with code %d", res.ExitCode)
	}
	if _, err := os.Stat(outPath); err != nil {
		return "", fmt.Errorf("powercfg reported success but the report file was not created")
	}
	return outPath, nil
}
