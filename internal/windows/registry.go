//go:build windows

package windows

import (
	"errors"
	"fmt"
	"syscall"

	"github.com/developerasaad/winprep/internal/platform"
	"golang.org/x/sys/windows/registry"
)

// RegistryReader implements platform.Registry with the x/sys registry package.
// It opens keys with read access only and never writes.
type RegistryReader struct{}

// NewRegistry returns the Windows registry reader.
func NewRegistry() *RegistryReader { return &RegistryReader{} }

func rootKeyOf(root platform.RootKey) (registry.Key, error) {
	switch root {
	case platform.HKLM:
		return registry.LOCAL_MACHINE, nil
	case platform.HKU:
		return registry.USERS, nil
	default:
		return 0, fmt.Errorf("unsupported registry root %d", root)
	}
}

func (r *RegistryReader) open(rootKey platform.RootKey, path string) (registry.Key, error) {
	root, err := rootKeyOf(rootKey)
	if err != nil {
		return 0, err
	}
	k, err := registry.OpenKey(root, path, registry.QUERY_VALUE)
	if err != nil {
		return 0, translateErr(err)
	}
	return k, nil
}

// translateErr converts the two "not found" errors from the registry package
// into platform.ErrValueNotFound so callers can distinguish missing values
// from real failures.
func translateErr(err error) error {
	if errors.Is(err, registry.ErrNotExist) || errors.Is(err, syscall.ERROR_FILE_NOT_FOUND) {
		return platform.ErrValueNotFound
	}
	return err
}

func (r *RegistryReader) StringValue(root platform.RootKey, path, name string) (string, bool, error) {
	k, err := r.open(root, path)
	if err != nil {
		if errors.Is(err, platform.ErrValueNotFound) {
			return "", false, nil
		}
		return "", false, err
	}
	defer k.Close()
	v, _, err := k.GetStringValue(name)
	if err != nil {
		if errors.Is(err, platform.ErrValueNotFound) || errors.Is(err, registry.ErrNotExist) || errors.Is(err, syscall.ERROR_FILE_NOT_FOUND) {
			return "", false, nil
		}
		return "", false, fmt.Errorf("reading %s\\%s\\%s: %w", root, path, name, err)
	}
	return v, true, nil
}

func (r *RegistryReader) DWORDValue(root platform.RootKey, path, name string) (uint32, bool, error) {
	k, err := r.open(root, path)
	if err != nil {
		if errors.Is(err, platform.ErrValueNotFound) {
			return 0, false, nil
		}
		return 0, false, err
	}
	defer k.Close()
	v, _, err := k.GetIntegerValue(name)
	if err != nil {
		if errors.Is(err, registry.ErrNotExist) || errors.Is(err, syscall.ERROR_FILE_NOT_FOUND) {
			return 0, false, nil
		}
		return 0, false, fmt.Errorf("reading %s\\%s\\%s: %w", root, path, name, err)
	}
	return uint32(v), true, nil
}

func (r *RegistryReader) MultiStringValue(root platform.RootKey, path, name string) ([]string, bool, error) {
	k, err := r.open(root, path)
	if err != nil {
		if errors.Is(err, platform.ErrValueNotFound) {
			return nil, false, nil
		}
		return nil, false, err
	}
	defer k.Close()
	v, _, err := k.GetStringsValue(name)
	if err != nil {
		if errors.Is(err, registry.ErrNotExist) || errors.Is(err, syscall.ERROR_FILE_NOT_FOUND) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("reading %s\\%s\\%s: %w", root, path, name, err)
	}
	return v, true, nil
}

func (r *RegistryReader) KeyExists(root platform.RootKey, path string) (bool, error) {
	rootKey, err := rootKeyOf(root)
	if err != nil {
		return false, err
	}
	k, err := registry.OpenKey(rootKey, path, registry.QUERY_VALUE)
	if err != nil {
		if errors.Is(err, registry.ErrNotExist) || errors.Is(err, syscall.ERROR_FILE_NOT_FOUND) {
			return false, nil
		}
		return false, fmt.Errorf("opening %s\\%s: %w", root, path, err)
	}
	k.Close()
	return true, nil
}

func (r *RegistryReader) SubKeyNames(root platform.RootKey, path string) ([]string, error) {
	rootKey, err := rootKeyOf(root)
	if err != nil {
		return nil, err
	}
	k, err := registry.OpenKey(rootKey, path, registry.ENUMERATE_SUB_KEYS)
	if err != nil {
		return nil, fmt.Errorf("opening %s\\%s: %w", root, path, translateErr(err))
	}
	defer k.Close()
	names, err := k.ReadSubKeyNames(-1)
	if err != nil {
		return nil, fmt.Errorf("listing %s\\%s: %w", root, path, err)
	}
	return names, nil
}
