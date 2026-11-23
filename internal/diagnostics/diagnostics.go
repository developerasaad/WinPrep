// Package diagnostics collects the "Check this laptop" report: computer
// details, storage, battery, device problems, and activation state. All
// queries are read-only. Anything Windows cannot report is shown as
// unavailable with a reason, never as a fake value.
package diagnostics

import (
	"context"
	"fmt"

	"github.com/developerasaad/winprep/internal/format"
	"github.com/developerasaad/winprep/internal/platform"
)

// Line is one key/value row in a report section. When Available is false the
// Value is empty and Note explains why the information could not be provided.
type Line struct {
	Label     string
	Value     string
	Available bool
	Note      string
}

// Section is a titled group of lines.
type Section struct {
	Title string
	Lines []Line
}

// Report is the full diagnostic result.
type Report struct {
	Sections []Section
}

// Service assembles diagnostics from the platform services.
type Service struct {
	Sys platform.System
	Reg platform.Registry
	Run platform.Runner

	// PSPath is the absolute path of powershell.exe. The hardware query runs
	// through Windows PowerShell and its stable, language-neutral JSON
	// output.
	PSPath string
	// PowerCfgPath is the absolute path of powercfg.exe for battery reports.
	PowerCfgPath string
}

// NewService returns a diagnostics service.
func NewService(sys platform.System, run platform.Runner, psPath, powerCfgPath string) *Service {
	return &Service{Sys: sys, Run: run, PSPath: psPath, PowerCfgPath: powerCfgPath}
}

// Inspect runs all diagnostic queries and assembles the report. A failure of
// the overall hardware query is reflected as unavailable sections, not as an
// error: the system facts are still worth reporting. An error is returned
// only when even the basic system facts cannot be read.
func (s *Service) Inspect(ctx context.Context) (Report, error) {
	info, err := s.Sys.SystemInfo(ctx)
	if err != nil {
		return Report{}, fmt.Errorf("reading system information: %w", err)
	}

	report := Report{Sections: []Section{}}
	report.Sections = append(report.Sections, computerSection(info))

	raw, rawErr := s.queryHardware(ctx)
	if rawErr != nil {
		note := "Not available: " + rawErr.Error() + ". This does not mean the hardware is faulty; Windows could not be queried."
		report.Sections = append(report.Sections,
			Section{Title: "Storage", Lines: []Line{{Label: "Storage devices", Note: note}}},
			Section{Title: "Battery", Lines: []Line{{Label: "Battery", Note: note}}},
			Section{Title: "Devices with problems", Lines: []Line{{Label: "Device status", Note: note}}},
			Section{Title: "Windows activation", Lines: []Line{{Label: "Activation state", Note: note}}},
		)
		return report, nil
	}

	report.Sections = append(report.Sections,
		storageSection(raw),
		batterySection(raw),
		devicesSection(raw),
		activationSection(raw),
	)
	return report, nil
}

func computerSection(info platform.SystemInfo) Section {
	lines := []Line{
		{Label: "Windows", Value: info.OSName, Available: info.OSName != ""},
		{Label: "Build", Value: info.Build, Available: info.Build != ""},
	}
	if info.DisplayVersion != "" {
		lines = append(lines, Line{Label: "Version", Value: info.DisplayVersion, Available: true})
	}
	lines = append(lines,
		Line{Label: "Manufacturer", Value: info.Manufacturer, Available: info.Manufacturer != ""},
		Line{Label: "Model", Value: info.Model, Available: info.Model != ""},
		Line{Label: "Processor", Value: info.CPU, Available: info.CPU != ""},
	)
	if info.TotalRAMBytes > 0 {
		lines = append(lines, Line{Label: "Memory (RAM)", Value: format.Bytes(info.TotalRAMBytes), Available: true})
	} else {
		lines = append(lines, Line{Label: "Memory (RAM)", Note: "Windows did not report the installed memory."})
	}
	return Section{Title: "Computer", Lines: lines}
}

// Section titles used by the report; exported for tests and the UI.
const (
	SectionComputer   = "Computer"
	SectionStorage    = "Storage"
	SectionBattery    = "Battery"
	SectionDevices    = "Devices with problems"
	SectionActivation = "Windows activation"
)
