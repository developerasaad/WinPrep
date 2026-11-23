package audit

import (
	"context"
	"errors"

	"github.com/developerasaad/winprep/internal/platform"
)

// Test helpers shared by this package's tests. They use the generic fakes so
// tests exercise the same types production code consumes.

func newFailingRegistry() *failRegistry { return &failRegistry{} }

type failRegistry struct{}

func (f *failRegistry) StringValue(platform.RootKey, string, string) (string, bool, error) {
	return "", false, errors.New("registry unavailable")
}

func (f *failRegistry) DWORDValue(platform.RootKey, string, string) (uint32, bool, error) {
	return 0, false, errors.New("registry unavailable")
}

func (f *failRegistry) MultiStringValue(platform.RootKey, string, string) ([]string, bool, error) {
	return nil, false, errors.New("registry unavailable")
}

func (f *failRegistry) KeyExists(platform.RootKey, string) (bool, error) {
	return false, errors.New("registry unavailable")
}

func (f *failRegistry) SubKeyNames(platform.RootKey, string) ([]string, error) {
	return nil, errors.New("registry unavailable")
}

type fakeSystem struct {
	usr platform.UserInfo
	err error
}

func (f *fakeSystem) SystemInfo(context.Context) (platform.SystemInfo, error) {
	if f.err != nil {
		return platform.SystemInfo{}, f.err
	}
	return platform.SystemInfo{IsWindows: true}, nil
}

func (f *fakeSystem) UserInfo(context.Context) (platform.UserInfo, error) {
	if f.err != nil {
		return platform.UserInfo{}, f.err
	}
	return f.usr, nil
}

func adminUser() platform.UserInfo {
	return platform.UserInfo{
		Name:           `WKS\Administrator`,
		SID:            "S-1-5-21-1-2-3-500",
		IsElevated:     true,
		IsBuiltinAdmin: true,
	}
}
