package diagnostics

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/developerasaad/winprep/internal/format"
)

// hardwareScript is the single PowerShell query behind the hardware sections.
// One invocation keeps the check fast on low-end hardware. Output is forced
// to UTF-8 and JSON so nothing depends on localized command-line text.
const hardwareScript = `[Console]::OutputEncoding = [System.Text.Encoding]::UTF8
$ErrorActionPreference = 'SilentlyContinue'
$result = [pscustomobject]@{
  physicalDisks = @(Get-PhysicalDisk | ForEach-Object { [pscustomobject]@{
    DeviceId = [string]$_.DeviceId; FriendlyName = [string]$_.FriendlyName
    MediaType = [string]$_.MediaType; BusType = [string]$_.BusType
    HealthStatus = [string]$_.HealthStatus; SizeBytes = [uint64]$_.Size } })
  diskDrives = @(Get-CimInstance -ClassName Win32_DiskDrive | ForEach-Object { [pscustomobject]@{
    Index = [int]$_.Index; Model = [string]$_.Model; SizeBytes = [uint64]$_.Size; MediaType = [string]$_.MediaType } })
  windowsDisk = $null
  battery = @(Get-CimInstance -ClassName Win32_Battery | ForEach-Object { [pscustomobject]@{
    Name = [string]$_.Name } })
  batteryDesigned = $null
  batteryFullCharge = $null
  batteryCycles = $null
  problemDevices = @(Get-CimInstance -ClassName Win32_PnPEntity -Filter 'ConfigManagerErrorCode <> 0' | ForEach-Object { [pscustomobject]@{
    Name = [string]$_.Name; DeviceId = [string]$_.DeviceID; Code = [int]$_.ConfigManagerErrorCode } })
  activation = @(Get-CimInstance -ClassName SoftwareLicensingProduct -Filter "ApplicationID='55c92734-d682-4d71-983e-d6ec3f16059f' AND PartialProductKey IS NOT NULL" | ForEach-Object { [pscustomobject]@{
    LicenseStatus = [int]$_.LicenseStatus; Edition = [string]$_.Name } })
}
$sysDrive = ($env:SystemDrive -replace ':$','')
$part = Get-Partition -DriveLetter $sysDrive | Select-Object -First 1
if ($part) { $result.windowsDisk = [int]$part.DiskNumber }
$static = Get-CimInstance -Namespace root/wmi -ClassName BatteryStaticData | Select-Object -First 1
if ($static) { $result.batteryDesigned = [uint64]$static.DesignedCapacity }
$full = Get-CimInstance -Namespace root/wmi -ClassName BatteryFullChargedCapacity | Select-Object -First 1
if ($full) { $result.batteryFullCharge = [uint64]$full.FullChargedCapacity }
$cycles = Get-CimInstance -Namespace root/wmi -ClassName BatteryCycleCount | Select-Object -First 1
if ($cycles) { $result.batteryCycles = [uint64]$cycles.CycleCount }
$result | ConvertTo-Json -Depth 4`

// rawDiagnostics mirrors the JSON produced by hardwareScript.
type rawDiagnostics struct {
	PhysicalDisks []rawPhysicalDisk `json:"physicalDisks"`
	DiskDrives    []rawDiskDrive    `json:"diskDrives"`
	WindowsDisk   *int              `json:"windowsDisk"`
	Battery       []rawBattery      `json:"battery"`
	// The battery numbers are pointers so an explicit JSON null is
	// distinguishable from zero.
	BatteryDesigned   *uint64            `json:"batteryDesigned"`
	BatteryFullCharge *uint64            `json:"batteryFullCharge"`
	BatteryCycles     *uint64            `json:"batteryCycles"`
	ProblemDevices    []rawProblemDevice `json:"problemDevices"`
	Activation        []rawActivation    `json:"activation"`
}

type rawPhysicalDisk struct {
	DeviceID     string `json:"DeviceId"`
	FriendlyName string `json:"FriendlyName"`
	MediaType    string `json:"MediaType"`
	BusType      string `json:"BusType"`
	HealthStatus string `json:"HealthStatus"`
	SizeBytes    uint64 `json:"SizeBytes"`
}

type rawDiskDrive struct {
	Index     int    `json:"Index"`
	Model     string `json:"Model"`
	SizeBytes uint64 `json:"SizeBytes"`
	MediaType string `json:"MediaType"`
}

type rawBattery struct {
	Name string `json:"Name"`
}

type rawProblemDevice struct {
	Name     string `json:"Name"`
	DeviceID string `json:"DeviceId"`
	Code     int    `json:"Code"`
}

type rawActivation struct {
	LicenseStatus int    `json:"LicenseStatus"`
	Edition       string `json:"Edition"`
}

func (s *Service) queryHardware(ctx context.Context) (rawDiagnostics, error) {
	var raw rawDiagnostics
	res, err := s.Run.Run(ctx, s.PSPath, "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", hardwareScript)
	if err != nil {
		return raw, fmt.Errorf("starting the hardware query: %w", err)
	}
	if res.ExitCode != 0 {
		return raw, fmt.Errorf("the hardware query failed with exit code %d", res.ExitCode)
	}
	out := trimJSONPrefix(res.Stdout)
	if out == "" {
		return raw, fmt.Errorf("the hardware query returned no data")
	}
	if err := json.Unmarshal([]byte(out), &raw); err != nil {
		return raw, fmt.Errorf("the hardware query returned data WinPrep could not interpret")
	}
	return raw, nil
}

// trimJSONPrefix drops any stray output before the first JSON object, which
// some systems emit (for example, storage driver warnings).
func trimJSONPrefix(s string) string {
	for i := 0; i < len(s); i++ {
		if s[i] == '{' {
			return s[i:]
		}
	}
	return ""
}

func storageSection(raw rawDiagnostics) Section {
	lines := []Line{}
	disks := raw.PhysicalDisks
	usingFallback := false
	if len(disks) == 0 && len(raw.DiskDrives) > 0 {
		// Get-PhysicalDisk was unavailable; fall back to Win32_DiskDrive,
		// which lacks health data. Media type strings differ between the two
		// sources and are normalized below.
		usingFallback = true
		for _, d := range raw.DiskDrives {
			disks = append(disks, rawPhysicalDisk{
				DeviceID:     strconv.Itoa(d.Index),
				FriendlyName: d.Model,
				MediaType:    d.MediaType,
				SizeBytes:    d.SizeBytes,
			})
		}
	}

	if len(disks) == 0 {
		return Section{Title: SectionStorage, Lines: []Line{{
			Label: "Storage devices",
			Note:  "No storage devices were reported. This can happen on systems with restricted storage queries and does not by itself mean the disk is missing or broken.",
		}}}
	}

	windowsDisk := ""
	if raw.WindowsDisk != nil {
		windowsDisk = strconv.Itoa(*raw.WindowsDisk)
	}

	for i, d := range disks {
		label := fmt.Sprintf("Disk %d", i)
		value := ""
		if d.FriendlyName != "" {
			value = d.FriendlyName
		} else {
			value = "model not reported"
		}
		if d.SizeBytes > 0 {
			value += ", " + format.Bytes(d.SizeBytes)
		}
		value += ", " + mediaTypeText(d.MediaType)
		if d.BusType != "" && !usingFallback {
			value += " (" + d.BusType + ")"
		}
		if windowsDisk != "" && d.DeviceID == windowsDisk {
			value += " - Windows is installed here"
		} else if windowsDisk == "" {
			// Never guess which disk carries Windows.
			valueNote := " (could not determine whether Windows is installed here)"
			if i == 0 {
				value += valueNote
			}
		}
		lines = append(lines, Line{Label: label, Value: value, Available: true})

		if usingFallback {
			lines = append(lines, Line{Label: "  Health", Note: "Health information requires the storage query, which was unavailable here. A missing health reading is not evidence that the disk is healthy."})
		} else {
			lines = append(lines, Line{Label: "  Health", Value: healthText(d.HealthStatus), Available: d.HealthStatus != ""})
		}
	}
	if usingFallback {
		lines = append(lines, Line{Label: "Note", Value: "Detailed storage health was not available from Windows on this machine.", Available: true})
	}
	if windowsDisk == "" {
		lines = append(lines, Line{Label: "Note", Value: "The disk carrying Windows could not be identified, so disk roles above are not confirmed.", Available: true})
	}
	return Section{Title: SectionStorage, Lines: lines}
}

// mediaTypeText normalizes media type strings from both storage sources into
// something a refurbisher can act on.
func mediaTypeText(s string) string {
	switch s {
	case "":
		return "media type not reported"
	case "SSD", "HDD":
		return s
	case "Unspecified", "Unknown":
		return "media type not reported"
	default:
		// Win32_DiskDrive strings such as "Fixed hard disk media".
		return "media type: " + s
	}
}

// healthText translates storage health status without over-claiming: only
// "Healthy" means healthy, and even that is the drive's own report, not a
// guarantee.
func healthText(s string) string {
	switch s {
	case "Healthy":
		return "Reported healthy (self-reported by the drive)"
	case "Warning":
		return "Warning - the drive reported a degraded state"
	case "Unhealthy":
		return "Unhealthy - the drive reported a failing state"
	case "":
		return ""
	default:
		return "Status reported as: " + s
	}
}

func batterySection(raw rawDiagnostics) Section {
	hasBattery := len(raw.Battery) > 0

	if !hasBattery && raw.BatteryDesigned == nil && raw.BatteryFullCharge == nil {
		return Section{Title: SectionBattery, Lines: []Line{{
			Label: "Battery",
			Note:  "No battery was detected. This machine may not have one, or Windows may not be able to see it. No health percentage is shown because there is nothing to measure.",
		}}}
	}

	lines := []Line{}
	if name := raw.batteryName(); name != "" {
		lines = append(lines, Line{Label: "Battery", Value: name, Available: true})
	} else {
		lines = append(lines, Line{Label: "Battery", Value: "Present", Available: true})
	}

	switch {
	case raw.BatteryDesigned != nil && *raw.BatteryDesigned > 0:
		lines = append(lines, Line{Label: "Design capacity", Value: format.MilliWattHours(*raw.BatteryDesigned), Available: true})
	case hasBattery:
		lines = append(lines, Line{Label: "Design capacity", Note: "The battery did not report its design capacity, so health cannot be calculated. No percentage is shown rather than a made-up value."})
	default:
		lines = append(lines, Line{Label: "Design capacity", Note: "The battery did not report its design capacity, so health cannot be calculated. No percentage is shown rather than a made-up value."})
	}

	switch {
	case raw.BatteryFullCharge != nil && *raw.BatteryFullCharge > 0:
		lines = append(lines, Line{Label: "Full-charge capacity", Value: format.MilliWattHours(*raw.BatteryFullCharge), Available: true})
	default:
		lines = append(lines, Line{Label: "Full-charge capacity", Note: "The battery did not report its current full-charge capacity."})
	}

	if raw.BatteryDesigned != nil && raw.BatteryFullCharge != nil &&
		*raw.BatteryDesigned > 0 && *raw.BatteryFullCharge > 0 {
		pct := float64(*raw.BatteryFullCharge) / float64(*raw.BatteryDesigned) * 100
		health := fmt.Sprintf("%.0f%% of design capacity", pct)
		if pct > 100 {
			health += " (above design; battery firmware data may be inaccurate)"
		}
		lines = append(lines, Line{Label: "Estimated health", Value: health, Available: true})
	}

	if raw.BatteryCycles != nil {
		lines = append(lines, Line{Label: "Cycle count", Value: fmt.Sprintf("%d cycles", *raw.BatteryCycles), Available: true})
	} else {
		lines = append(lines, Line{Label: "Cycle count", Note: "This battery did not report a cycle count."})
	}

	lines = append(lines, Line{Label: "Full report", Value: "A detailed Windows battery report can be generated from this screen or with: winprep check --battery-report", Available: true})
	return Section{Title: SectionBattery, Lines: lines}
}

func (r rawDiagnostics) batteryName() string {
	if len(r.Battery) == 0 {
		return ""
	}
	return r.Battery[0].Name
}

func devicesSection(raw rawDiagnostics) Section {
	if len(raw.ProblemDevices) == 0 {
		return Section{Title: SectionDevices, Lines: []Line{{
			Label:     "Device status",
			Value:     "No devices with problems were detected. This reflects Device Manager status only; it does not replace a physical check.",
			Available: true,
		}}}
	}
	lines := []Line{}
	for _, d := range raw.ProblemDevices {
		name := d.Name
		if name == "" {
			name = d.DeviceID
		}
		lines = append(lines, Line{Label: name, Value: CodeManagerText(d.Code), Available: true})
	}
	lines = append(lines, Line{Label: "Note", Value: "WinPrep does not install or update drivers. Resolve these in Device Manager or with the manufacturer's driver package.", Available: true})
	return Section{Title: SectionDevices, Lines: lines}
}

// CodeManagerText maps Device Manager problem codes to plain descriptions.
// The numeric code is always included so the message stays useful for codes
// not in the map.
func CodeManagerText(code int) string {
	descriptions := map[int]string{
		1:  "driver missing or not configured",
		3:  "driver may be corrupted or memory is low",
		10: "device cannot start",
		12: "not enough resources to start the device",
		14: "device needs a restart to finish setup",
		18: "driver needs to be reinstalled",
		19: "configuration data in the registry is damaged",
		21: "Windows is removing this device",
		22: "device is disabled",
		24: "device not present or not working (often a driver or hardware fault)",
		28: "drivers are not installed",
		31: "device is not working properly",
		37: "Windows cannot initialize the driver for this device",
		39: "driver is missing or corrupted",
		43: "device reported a problem and was stopped (often a hardware fault)",
		45: "device is not currently connected (recorded problem)",
	}
	if d, ok := descriptions[code]; ok {
		return fmt.Sprintf("Problem code %d: %s", code, d)
	}
	return fmt.Sprintf("Problem code %d", code)
}

func activationSection(raw rawDiagnostics) Section {
	if len(raw.Activation) == 0 {
		return Section{Title: SectionActivation, Lines: []Line{{
			Label: "Activation state",
			Note:  "Windows did not report an activation state. WinPrep does not attempt activation changes of any kind.",
		}}}
	}
	a := raw.Activation[0]
	return Section{Title: SectionActivation, Lines: []Line{
		{Label: "Activation state", Value: ActivationText(a.LicenseStatus), Available: true},
		// The edition string comes from SoftwareLicensingProduct and never
		// contains product key material.
		{Label: "Edition", Value: a.Edition, Available: a.Edition != ""},
		{Label: "Product key", Value: "WinPrep never displays product keys.", Available: true},
	}}
}

// ActivationText maps SLS LicenseStatus values to plain descriptions.
func ActivationText(status int) string {
	switch status {
	case 0:
		return "Unlicensed"
	case 1:
		return "Activated (licensed)"
	case 2:
		return "In out-of-box grace period (not yet activated)"
	case 3:
		return "In out-of-tolerance grace period (activation issue)"
	case 4:
		return "In non-genuine grace period (Windows reports a licensing problem)"
	case 5:
		return "Not activated (notification mode)"
	case 6:
		return "In extended grace period (not yet activated)"
	default:
		return fmt.Sprintf("Reported status code %d", status)
	}
}
