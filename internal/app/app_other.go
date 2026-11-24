//go:build !windows

package app

import (
	"context"
	"errors"
	"time"

	"github.com/developerasaad/winprep/internal/artifacts"
	"github.com/developerasaad/winprep/internal/handover"
	"github.com/developerasaad/winprep/internal/platform"
)

// New assembles the application for non-Windows builds. WinPrep targets
// Windows, so every platform service reports ErrUnsupportedPlatform. This
// exists so the tool can be developed, unit-tested, and built
// cross-platform; on any other OS it will explain that it cannot run here
// rather than produce misleading output.
func New() *App {
	stub := &stubSystem{}
	return Assemble(
		stub,
		&stubRegistry{},
		&stubRunner{},
		&stubProcessLister{},
		artifacts.NewStore("."),
		&stubSysprep{},
		&stubSysprepMonitor{},
		&stubInstanceLock{},
		"",
	)
}

type stubSystem struct{}

func (s *stubSystem) SystemInfo(context.Context) (platform.SystemInfo, error) {
	return platform.SystemInfo{}, platform.ErrUnsupportedPlatform
}

func (s *stubSystem) UserInfo(context.Context) (platform.UserInfo, error) {
	return platform.UserInfo{}, platform.ErrUnsupportedPlatform
}

func (s *stubSystem) LocalAccounts(context.Context) ([]platform.LocalAccount, error) {
	return nil, platform.ErrUnsupportedPlatform
}

type stubRegistry struct{}

func (r *stubRegistry) StringValue(platform.RootKey, string, string) (string, bool, error) {
	return "", false, platform.ErrUnsupportedPlatform
}

func (r *stubRegistry) DWORDValue(platform.RootKey, string, string) (uint32, bool, error) {
	return 0, false, platform.ErrUnsupportedPlatform
}

func (r *stubRegistry) MultiStringValue(platform.RootKey, string, string) ([]string, bool, error) {
	return nil, false, platform.ErrUnsupportedPlatform
}

func (r *stubRegistry) KeyExists(platform.RootKey, string) (bool, error) {
	return false, platform.ErrUnsupportedPlatform
}

func (r *stubRegistry) SubKeyNames(platform.RootKey, string) ([]string, error) {
	return nil, platform.ErrUnsupportedPlatform
}

type stubRunner struct{}

func (r *stubRunner) RunVisible(context.Context, string, ...string) (int, error) {
	return 0, platform.ErrUnsupportedPlatform
}

type stubProcessLister struct{}

func (p *stubProcessLister) ProcessNames(context.Context) ([]string, error) {
	return nil, platform.ErrUnsupportedPlatform
}

type stubSysprepMonitor struct{}

func (m *stubSysprepMonitor) SysprepLogTimes(context.Context) ([]time.Time, error) {
	return nil, platform.ErrUnsupportedPlatform
}

type stubInstanceLock struct{}

func (l *stubInstanceLock) TryLock() (bool, error) {
	return false, platform.ErrUnsupportedPlatform
}

type stubSysprep struct{}

func (s *stubSysprep) SysprepPath() (string, error) {
	return "", errors.New("sysprep is only available on Windows")
}

func (s *stubSysprep) RunReseal(context.Context) (int, error) {
	return 0, platform.ErrUnsupportedPlatform
}

// Compile-time check that the handover flow stays constructible with stubs
// (used by non-Windows unit tests).
var _ handover.SysprepRunner = (*stubSysprep)(nil)
