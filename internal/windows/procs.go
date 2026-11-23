//go:build windows

package windows

import (
	"context"
	"fmt"
	"unsafe"

	xwin "golang.org/x/sys/windows"
)

// ProcessLister implements platform.ProcessLister with a tool help snapshot.
// It avoids spawning tasklist.exe, whose output is localized and therefore
// fragile to parse.
type ProcessLister struct{}

// NewProcessLister returns the native Windows process lister.
func NewProcessLister() *ProcessLister { return &ProcessLister{} }

// ProcessNames implements platform.ProcessLister.
func (p *ProcessLister) ProcessNames(context.Context) ([]string, error) {
	snap, err := xwin.CreateToolhelp32Snapshot(xwin.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil, fmt.Errorf("creating process snapshot: %w", err)
	}
	defer xwin.CloseHandle(snap)

	var pe xwin.ProcessEntry32
	pe.Size = uint32(unsafe.Sizeof(pe))
	names := make([]string, 0, 128)
	if err := xwin.Process32First(snap, &pe); err != nil {
		// An empty snapshot is not an error worth failing on.
		return names, nil
	}
	for {
		names = append(names, xwin.UTF16ToString(pe.ExeFile[:]))
		if err := xwin.Process32Next(snap, &pe); err != nil {
			break
		}
	}
	return names, nil
}
