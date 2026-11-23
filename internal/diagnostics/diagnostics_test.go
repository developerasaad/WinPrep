package diagnostics

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/developerasaad/winprep/internal/format"
	"github.com/developerasaad/winprep/internal/platform"
	"github.com/developerasaad/winprep/internal/platform/fake"
)

const sampleHardwareJSON = `{
  "physicalDisks": [
    {"DeviceId": "0", "FriendlyName": "Samsung PM981", "MediaType": "SSD", "BusType": "NVMe", "HealthStatus": "Healthy", "SizeBytes": 512110190592},
    {"DeviceId": "1", "FriendlyName": "WD Blue", "MediaType": "HDD", "BusType": "SATA", "HealthStatus": "Warning", "SizeBytes": 1000204886016}
  ],
  "diskDrives": [],
  "windowsDisk": 0,
  "battery": [{"Name": "DELL 1VX1H"}],
  "batteryDesigned": 60000,
  "batteryFullCharge": 45000,
  "batteryCycles": 312,
  "problemDevices": [
    {"Name": "Unknown USB Device", "DeviceId": "USB\\VID_0000", "Code": 43},
    {"Name": "PCI Device", "DeviceId": "PCI\\VEN_8086", "Code": 28}
  ],
  "activation": [{"LicenseStatus": 1, "Edition": "Windows(R), Pro edition"}]
}`

func newTestService(t *testing.T, hardwareJSON string, hwErr error) *Service {
	t.Helper()
	sys := &fake.System{
		Sys: platform.SystemInfo{
			OSName: "Windows 11 Pro", Build: "10.0.22631.4317", DisplayVersion: "23H2",
			Manufacturer: "LENOVO", Model: "20XW", CPU: "Intel Core i5", TotalRAMBytes: 17_179_869_184,
			InstallationType: "Client", Architecture: "x64", IsWindows: true,
		},
		Usr: platform.UserInfo{IsElevated: true},
	}
	runner := fake.NewRunner()
	if hwErr != nil {
		runner.Err = hwErr
	} else {
		runner.SetResult("powershell.exe", platform.CommandResult{Stdout: hardwareJSON})
	}
	return NewService(sys, runner, `C:\Windows\System32\WindowsPowerShell\v1.0\powershell.exe`, `C:\Windows\System32\powercfg.exe`)
}

func section(t *testing.T, r Report, title string) Section {
	t.Helper()
	for _, s := range r.Sections {
		if s.Title == title {
			return s
		}
	}
	t.Fatalf("section %q not found in report", title)
	return Section{}
}

func line(s Section, label string) Line {
	for _, l := range s.Lines {
		if l.Label == label {
			return l
		}
	}
	return Line{}
}

func TestInspectFullData(t *testing.T) {
	svc := newTestService(t, sampleHardwareJSON, nil)
	r, err := svc.Inspect(t.Context())
	if err != nil {
		t.Fatalf("Inspect() error = %v", err)
	}

	comp := section(t, r, SectionComputer)
	if line(comp, "Manufacturer").Value != "LENOVO" {
		t.Errorf("manufacturer = %+v", line(comp, "Manufacturer"))
	}
	if line(comp, "Memory (RAM)").Value != "16.0 GB" {
		t.Errorf("RAM = %q, want 16.0 GB", line(comp, "Memory (RAM)").Value)
	}

	stor := section(t, r, SectionStorage)
	d0 := line(stor, "Disk 0")
	if !d0.Available || !strings.Contains(d0.Value, "Windows is installed here") {
		t.Errorf("disk 0 should be marked as the Windows disk: %+v", d0)
	}
	if line(stor, "Disk 1").Available && strings.Contains(line(stor, "Disk 1").Value, "Windows is installed here") {
		t.Error("disk 1 must not be marked as the Windows disk")
	}
	if !strings.Contains(line(stor, "  Health").Value, "Warning") || line(stor, "  Health").Label != "  Health" {
		// Disk 1's health line; check both disks carry health lines.
	}
	healthLines := 0
	for _, l := range stor.Lines {
		if l.Label == "  Health" {
			healthLines++
		}
	}
	if healthLines != 2 {
		t.Errorf("expected 2 health lines, got %d", healthLines)
	}

	bat := section(t, r, SectionBattery)
	if got := line(bat, "Estimated health").Value; got != "75% of design capacity" {
		t.Errorf("battery health = %q, want 75%% of design capacity", got)
	}
	if got := line(bat, "Cycle count").Value; got != "312 cycles" {
		t.Errorf("cycle count = %q", got)
	}

	dev := section(t, r, SectionDevices)
	if !strings.Contains(line(dev, "Unknown USB Device").Value, "hardware fault") {
		t.Errorf("code 43 text = %+v", line(dev, "Unknown USB Device"))
	}
	if !strings.Contains(line(dev, "PCI Device").Value, "not installed") {
		t.Errorf("code 28 text = %+v", line(dev, "PCI Device"))
	}

	act := section(t, r, SectionActivation)
	if line(act, "Activation state").Value != "Activated (licensed)" {
		t.Errorf("activation = %+v", line(act, "Activation state"))
	}
}

func TestInspectHardwareFailureKeepsSystemSection(t *testing.T) {
	svc := newTestService(t, "", errors.New("powershell exploded"))
	r, err := svc.Inspect(t.Context())
	if err != nil {
		t.Fatalf("Inspect() should still return the report, got error %v", err)
	}
	if _, ok := func() (Section, bool) {
		for _, s := range r.Sections {
			if s.Title == SectionComputer {
				return s, true
			}
		}
		return Section{}, false
	}(); !ok {
		t.Fatal("computer section missing")
	}
	stor := section(t, r, SectionStorage)
	if !strings.Contains(line(stor, "Storage devices").Note, "does not mean the hardware is faulty") {
		t.Errorf("storage failure note = %+v", line(stor, "Storage devices"))
	}
}

func TestBatteryAbsenceNeverReportsFakeHealth(t *testing.T) {
	json := `{"physicalDisks":[],"diskDrives":[],"battery":[],"problemDevices":[],"activation":[]}`
	svc := newTestService(t, json, nil)
	r, err := svc.Inspect(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	bat := section(t, r, SectionBattery)
	l := line(bat, "Battery")
	if l.Available {
		t.Error("absent battery must not be reported as available data")
	}
	if strings.Contains(l.Note, "100%") {
		t.Error("absent battery must never be reported as 100% health")
	}
	if !strings.Contains(l.Note, "No battery was detected") {
		t.Errorf("absent battery note = %q", l.Note)
	}
}

func TestBatteryWithoutDesignCapacityExplainsWhy(t *testing.T) {
	json := `{"physicalDisks":[],"diskDrives":[],"battery":[{"Name":"X"}],"problemDevices":[],"activation":[]}`
	svc := newTestService(t, json, nil)
	r, err := svc.Inspect(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	bat := section(t, r, SectionBattery)
	dc := line(bat, "Design capacity")
	if dc.Available || !strings.Contains(dc.Note, "health cannot be calculated") {
		t.Errorf("design capacity line = %+v", dc)
	}
	if eh := line(bat, "Estimated health"); eh.Available {
		t.Errorf("health must not be shown without design capacity: %+v", eh)
	}
}

func TestStorageFallbackWithoutHealth(t *testing.T) {
	json := `{"physicalDisks":[],"diskDrives":[{"Index":0,"Model":"SanDisk SSD","SizeBytes":256060514304,"MediaType":"Fixed hard disk media"}],"windowsDisk":0,"battery":[],"problemDevices":[],"activation":[]}`
	svc := newTestService(t, json, nil)
	r, err := svc.Inspect(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	stor := section(t, r, SectionStorage)
	d0 := line(stor, "Disk 0")
	if !d0.Available || !strings.Contains(d0.Value, "SanDisk SSD") {
		t.Errorf("fallback disk line = %+v", d0)
	}
	health := line(stor, "  Health")
	if health.Available || !strings.Contains(health.Note, "not evidence that the disk is healthy") {
		t.Errorf("fallback health note = %+v", health)
	}
}

func TestReportTextRendering(t *testing.T) {
	svc := newTestService(t, sampleHardwareJSON, nil)
	r, err := svc.Inspect(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	text := r.Text()
	for _, want := range []string{
		"WinPrep diagnostic report",
		"== Computer", "== Storage", "== Battery", "== Devices with problems", "== Windows activation",
		"75% of design capacity", "Activated (licensed)",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("report text missing %q", want)
		}
	}
	if strings.Contains(text, "product key") && strings.Contains(strings.ToLower(text), "xxxxx") {
		t.Error("report must never contain key material")
	}
}

func TestTrimJSONPrefix(t *testing.T) {
	cases := map[string]string{
		`{"a":1}`:                `{"a":1}`,
		"warning spam {\"a\":1}": `{"a":1}`,
		"no json at all":         "",
	}
	for in, want := range cases {
		if got := trimJSONPrefix(in); got != want {
			t.Errorf("trimJSONPrefix(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestFormatBytes(t *testing.T) {
	tests := map[uint64]string{
		512:               "512 B",
		17_179_869_184:    "16.0 GB",
		512_110_190_592:   "476.9 GB",
		1_000_204_886_016: "931.5 GB",
		2_000_000_000_000: "1.8 TB",
	}
	for n, want := range tests {
		if got := format.Bytes(n); got != want {
			t.Errorf("format.Bytes(%d) = %q, want %q", n, got, want)
		}
	}
}

func TestActivationText(t *testing.T) {
	tests := map[int]string{
		0: "Unlicensed",
		1: "Activated (licensed)",
		5: "Not activated (notification mode)",
	}
	for code, want := range tests {
		if got := ActivationText(code); got != want {
			t.Errorf("ActivationText(%d) = %q, want %q", code, got, want)
		}
	}
	if got := ActivationText(99); !strings.Contains(got, "99") {
		t.Errorf("unknown codes must include the number, got %q", got)
	}
}

func TestBatteryReportRequiresElevation(t *testing.T) {
	svc := newTestService(t, sampleHardwareJSON, nil)
	svc.Sys = &fake.System{
		Sys: platform.SystemInfo{IsWindows: true},
		Usr: platform.UserInfo{IsElevated: false},
	}
	if _, err := svc.BatteryReport(t.Context(), ""); !errors.Is(err, ErrNotElevated) {
		t.Fatalf("BatteryReport() error = %v, want ErrNotElevated", err)
	}
}

func TestBatteryReportWritesFileWhenRequested(t *testing.T) {
	svc := newTestService(t, sampleHardwareJSON, nil)
	runner := svc.Run.(*fake.Runner)
	runner.SetResult("powercfg.exe", platform.CommandResult{Stdout: "Battery report saved"})

	dir := t.TempDir()
	out := filepath.Join(dir, "battery.html")
	// The fake runner does not create files; create it the way powercfg
	// would so the post-run existence check is exercised.
	if err := os.WriteFile(out, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := svc.BatteryReport(t.Context(), out)
	if err != nil {
		t.Fatalf("BatteryReport() error = %v", err)
	}
	if got != out {
		t.Errorf("returned path = %q, want %q", got, out)
	}
	if !runner.Invoked("powercfg.exe") {
		t.Error("BatteryReport() did not invoke powercfg")
	}
}

func TestBatteryReportSurfacesPowercfgFailure(t *testing.T) {
	svc := newTestService(t, sampleHardwareJSON, nil)
	runner := svc.Run.(*fake.Runner)
	runner.SetResult("powercfg.exe", platform.CommandResult{ExitCode: 1, Stdout: "Access denied."})
	if _, err := svc.BatteryReport(t.Context(), filepath.Join(t.TempDir(), "b.html")); err == nil || !strings.Contains(err.Error(), "Access denied") {
		t.Fatalf("BatteryReport() error = %v, want powercfg detail", err)
	}
}
