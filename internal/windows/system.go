//go:build windows

package windows

import (
	"context"
	"fmt"
	"runtime"
	"strconv"
	"strings"
	"unsafe"

	"github.com/developerasaad/winprep/internal/platform"
	xwin "golang.org/x/sys/windows"
)

// System implements platform.System for Windows. It answers from the registry
// and native API calls only; no external processes are spawned.
type System struct {
	Reg *RegistryReader
}

// NewSystem returns the Windows system inspector.
func NewSystem(reg *RegistryReader) *System { return &System{Reg: reg} }

const (
	cvKey      = `SOFTWARE\Microsoft\Windows NT\CurrentVersion`
	sysInfoKey = `SYSTEM\CurrentControlSet\Control\SystemInformation`
	biosKey    = `HARDWARE\DESCRIPTION\System\BIOS`
	cpuKey     = `HARDWARE\DESCRIPTION\System\CentralProcessor\0`
)

// SystemInfo implements platform.System. Individual facts degrade to empty
// strings rather than failing the whole query, because a partially readable
// system is still worth reporting; an error is returned only when something
// fundamental (such as the registry itself) is unavailable.
func (s *System) SystemInfo(context.Context) (platform.SystemInfo, error) {
	info := platform.SystemInfo{
		Architecture: archName(),
		IsWindows:    true,
	}

	// Windows edition and build. ProductName still reads "Windows 10 ..." on
	// many Windows 11 installs, so the marketing name is derived from the
	// build number instead and ProductName is only used for Server.
	productName, _, _ := s.Reg.StringValue(platform.HKLM, cvKey, "ProductName")
	editionID, _, _ := s.Reg.StringValue(platform.HKLM, cvKey, "EditionID")
	displayVersion, _, _ := s.Reg.StringValue(platform.HKLM, cvKey, "DisplayVersion")
	if displayVersion == "" {
		// Releases before 20H2 used ReleaseId (e.g. "2004").
		displayVersion, _, _ = s.Reg.StringValue(platform.HKLM, cvKey, "ReleaseId")
	}
	build, _, _ := s.Reg.StringValue(platform.HKLM, cvKey, "CurrentBuildNumber")
	ubr, ubrOK, _ := s.Reg.DWORDValue(platform.HKLM, cvKey, "UBR")
	installationType, _, _ := s.Reg.StringValue(platform.HKLM, cvKey, "InstallationType")

	info.Edition = editionName(editionID)
	info.DisplayVersion = displayVersion
	info.InstallationType = installationType
	if build != "" {
		if ubrOK {
			info.Build = fmt.Sprintf("10.0.%s.%d", build, ubr)
		} else {
			info.Build = "10.0." + build
		}
	}
	info.OSName = productNameFor(installationType, productName, build, info.Edition)

	info.Manufacturer, info.Model = s.readModel()
	info.CPU, _, _ = s.Reg.StringValue(platform.HKLM, cpuKey, "ProcessorNameString")
	info.TotalRAMBytes = totalPhysicalMemory()
	return info, nil
}

// readModel prefers the SystemInformation key (set by OEM deployment and
// readable without admin rights) and falls back to the BIOS key.
func (s *System) readModel() (string, string) {
	man, manOK, _ := s.Reg.StringValue(platform.HKLM, sysInfoKey, "SystemManufacturer")
	mod, modOK, _ := s.Reg.StringValue(platform.HKLM, sysInfoKey, "SystemProductName")
	if manOK || modOK {
		return man, mod
	}
	man, manOK, _ = s.Reg.StringValue(platform.HKLM, biosKey, "SystemManufacturer")
	mod, modOK, _ = s.Reg.StringValue(platform.HKLM, biosKey, "SystemProductName")
	if manOK || modOK {
		return man, mod
	}
	return "", ""
}

// UserInfo implements platform.System. It reads the process token directly,
// which is both faster and more reliable than parsing whoami output.
func (s *System) UserInfo(context.Context) (platform.UserInfo, error) {
	token := xwin.GetCurrentProcessToken()
	elevated := token.IsElevated()

	tu, err := token.GetTokenUser()
	if err != nil {
		return platform.UserInfo{}, fmt.Errorf("reading process token: %w", err)
	}
	sidPtr := tu.User.Sid
	sid := sidPtr.String()

	name := ""
	if acct, domain, _, err := sidPtr.LookupAccount(""); err == nil {
		if domain != "" {
			name = domain + `\` + acct
		} else {
			name = acct
		}
	}

	return platform.UserInfo{
		Name:           name,
		SID:            sid,
		IsElevated:     elevated,
		IsBuiltinAdmin: isBuiltinAdminSID(sid),
	}, nil
}

// isBuiltinAdminSID reports whether an SID is a machine-local
// S-1-5-21-...-500, i.e. the built-in Administrator account. Domain SIDs use
// the same -500 relative identifier, so the machine SID prefix matters only in
// that the account must be local (S-1-5-21 domain style, not well-known).
func isBuiltinAdminSID(sid string) bool {
	parts := strings.Split(sid, "-")
	return len(parts) == 8 && parts[1] == "1" && parts[2] == "5" && parts[3] == "21" && parts[7] == "500"
}

func totalPhysicalMemory() uint64 {
	var ms memoryStatusEx
	ms.dwLength = uint32(unsafe.Sizeof(ms))
	if r, _, _ := procGlobalMemoryStatusEx.Call(uintptr(unsafe.Pointer(&ms))); r != 0 {
		return ms.ullTotalPhys
	}
	return 0
}

// memoryStatusEx mirrors the MEMORYSTATUSEX structure used by
// GlobalMemoryStatusEx. x/sys/windows does not expose this API, so it is
// declared here directly.
type memoryStatusEx struct {
	dwLength                uint32
	dwMemoryLoad            uint32
	ullTotalPhys            uint64
	ullAvailPhys            uint64
	ullTotalPageFile        uint64
	ullAvailPageFile        uint64
	ullTotalVirtual         uint64
	ullAvailVirtual         uint64
	ullAvailExtendedVirtual uint64
}

var (
	modkernel32              = xwin.NewLazySystemDLL("kernel32.dll")
	procGlobalMemoryStatusEx = modkernel32.NewProc("GlobalMemoryStatusEx")
)

func archName() string {
	switch runtime.GOARCH {
	case "amd64":
		return "x64"
	case "arm64":
		return "ARM64"
	default:
		return runtime.GOARCH
	}
}

// productNameFor decides the display name. On client editions the registry's
// ProductName still reads "Windows 10 ..." on most Windows 11 installs, so
// the build number decides between 10 and 11; on Server the recorded
// ProductName is accurate and used verbatim.
func productNameFor(installationType, productName, build, edition string) string {
	if installationType == "Server" && productName != "" {
		return productName
	}
	base := "Windows"
	if buildNumberAtLeast(build, 22000) {
		base = "Windows 11"
	} else if buildNumberAtLeast(build, 10240) {
		base = "Windows 10"
	}
	if edition != "" {
		return base + " " + edition
	}
	return base
}

// buildNumberAtLeast reports whether the UBR part of a build string such as
// "10.0.22631" (or "10.0.22631.4317") is at least n.
func buildNumberAtLeast(build string, n int) bool {
	parts := strings.Split(build, ".")
	if len(parts) < 3 {
		return false
	}
	b, err := strconv.Atoi(parts[2])
	return err == nil && b >= n
}

// editionName maps common EditionID values to friendlier names. Unknown
// editions are returned unchanged.
func editionName(editionID string) string {
	switch editionID {
	case "":
		return ""
	case "Core":
		return "Home"
	case "CoreN":
		return "Home N"
	case "CoreSingleLanguage":
		return "Home Single Language"
	case "CoreCountrySpecific":
		return "Home (China)"
	case "Professional":
		return "Pro"
	case "ProfessionalN":
		return "Pro N"
	case "ProfessionalEducation":
		return "Pro Education"
	case "ProfessionalEducationN":
		return "Pro Education N"
	case "ProfessionalWorkstation":
		return "Pro for Workstations"
	case "ProfessionalWorkstationN":
		return "Pro N for Workstations"
	case "Education":
		return "Education"
	case "EducationN":
		return "Education N"
	case "Enterprise":
		return "Enterprise"
	case "EnterpriseN":
		return "Enterprise N"
	case "EnterpriseS":
		return "Enterprise LTSC"
	case "EnterpriseSN":
		return "Enterprise N LTSC"
	case "IoTEnterprise":
		return "IoT Enterprise"
	case "IoTEnterpriseS":
		return "IoT Enterprise LTSC"
	case "ServerRdsh":
		return "Enterprise for Virtual Desktops"
	default:
		return editionID
	}
}
