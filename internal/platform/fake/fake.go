// Package fake provides in-memory implementations of the platform interfaces
// for unit tests. It is a normal (non-test) package purely so tests in several
// packages can import it; nothing outside tests should depend on it.
package fake

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/developerasaad/winprep/internal/platform"
)

// Registry is an in-memory platform.Registry. Values are stored per path as
// either a string, uint32, or []string.
type Registry struct {
	mu        sync.Mutex
	strings   map[string]string
	dwords    map[string]uint32
	multi     map[string][]string
	keys      map[string]bool // existing keys, including keys that hold values
	subKeyErr map[string]error
	readErr   error // when set, all reads fail with this error
}

// NewRegistry returns an empty fake registry.
func NewRegistry() *Registry {
	return &Registry{
		strings:   map[string]string{},
		dwords:    map[string]uint32{},
		multi:     map[string][]string{},
		keys:      map[string]bool{},
		subKeyErr: map[string]error{},
	}
}

func keyPath(root platform.RootKey, path, name string) string {
	return root.String() + `\` + path + `\` + name
}

// SetString stores a REG_SZ value, creating its key.
func (r *Registry) SetString(root platform.RootKey, path, name, value string) *Registry {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.strings[keyPath(root, path, name)] = value
	r.keys[root.String()+`\`+path] = true
	return r
}

// SetDWORD stores a REG_DWORD value, creating its key.
func (r *Registry) SetDWORD(root platform.RootKey, path, name string, value uint32) *Registry {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.dwords[keyPath(root, path, name)] = value
	r.keys[root.String()+`\`+path] = true
	return r
}

// SetMultiString stores a REG_MULTI_SZ value, creating its key.
func (r *Registry) SetMultiString(root platform.RootKey, path, name string, value []string) *Registry {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.multi[keyPath(root, path, name)] = value
	r.keys[root.String()+`\`+path] = true
	return r
}

// AddKey marks a key as existing without any values.
func (r *Registry) AddKey(root platform.RootKey, path string) *Registry {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.keys[root.String()+`\`+path] = true
	return r
}

// FailSubKeys makes SubKeyNames return err for the given path.
func (r *Registry) FailSubKeys(root platform.RootKey, path string, err error) *Registry {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.subKeyErr[root.String()+`\`+path] = err
	return r
}

// FailReads makes every read return err.
func (r *Registry) FailReads(err error) *Registry {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.readErr = err
	return r
}

func (r *Registry) StringValue(root platform.RootKey, path, name string) (string, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.readErr != nil {
		return "", false, r.readErr
	}
	v, ok := r.strings[keyPath(root, path, name)]
	return v, ok, nil
}

func (r *Registry) DWORDValue(root platform.RootKey, path, name string) (uint32, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.readErr != nil {
		return 0, false, r.readErr
	}
	v, ok := r.dwords[keyPath(root, path, name)]
	return v, ok, nil
}

func (r *Registry) MultiStringValue(root platform.RootKey, path, name string) ([]string, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.readErr != nil {
		return nil, false, r.readErr
	}
	v, ok := r.multi[keyPath(root, path, name)]
	return v, ok, nil
}

func (r *Registry) KeyExists(root platform.RootKey, path string) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.readErr != nil {
		return false, r.readErr
	}
	return r.keys[root.String()+`\`+path], nil
}

func (r *Registry) SubKeyNames(root platform.RootKey, path string) ([]string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.readErr != nil {
		return nil, r.readErr
	}
	if err := r.subKeyErr[root.String()+`\`+path]; err != nil {
		return nil, err
	}
	prefix := root.String() + `\` + path + `\`
	seen := map[string]bool{}
	for k := range r.strings {
		if strings.HasPrefix(k, prefix) {
			rest := strings.TrimPrefix(k, prefix)
			if i := strings.Index(rest, `\`); i >= 0 {
				rest = rest[:i]
			}
			seen[rest] = true
		}
	}
	names := make([]string, 0, len(seen))
	for n := range seen {
		names = append(names, n)
	}
	sort.Strings(names)
	return names, nil
}

// Command records a single invocation made through a Runner.
type Command struct {
	Name      string
	Args      []string
	Visible   bool
	Cancelled bool
}

// Runner is an in-memory platform.Runner that records invocations and returns
// canned results matched by command name.
type Runner struct {
	mu       sync.Mutex
	Commands []Command
	// Results maps a lowercase command basename (e.g. "powershell.exe") to the
	// result returned for it. Missing entries produce an error.
	Results map[string]platform.CommandResult
	// scriptResults refines Results: when any argument of an invocation
	// contains the fragment, that result wins. It distinguishes the different
	// PowerShell queries, which all run through powershell.exe.
	scriptResults []scriptResult
	Err           error // returned for every command when set
}

type scriptResult struct {
	name     string
	fragment string
	res      platform.CommandResult
}

// NewRunner returns a fake runner that fails for unknown commands.
func NewRunner() *Runner {
	return &Runner{Results: map[string]platform.CommandResult{}}
}

// SetResult installs the result returned for the given command name.
func (r *Runner) SetResult(name string, res platform.CommandResult) *Runner {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.Results[strings.ToLower(name)] = res
	return r
}

// SetScriptResult installs a result returned when the command's basename
// matches name and any argument contains fragment. It takes precedence over
// SetResult, allowing different canned output for different PowerShell
// scripts.
func (r *Runner) SetScriptResult(name, fragment string, res platform.CommandResult) *Runner {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.scriptResults = append(r.scriptResults, scriptResult{
		name:     strings.ToLower(name),
		fragment: fragment,
		res:      res,
	})
	return r
}

func (r *Runner) run(visible bool, name string, args ...string) (platform.CommandResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.Commands = append(r.Commands, Command{Name: name, Args: append([]string(nil), args...), Visible: visible})
	if r.Err != nil {
		return platform.CommandResult{}, r.Err
	}
	base := strings.ToLower(name)
	if i := strings.LastIndex(base, `\`); i >= 0 {
		base = base[i+1:]
	}
	for _, sr := range r.scriptResults {
		if sr.name != base {
			continue
		}
		for _, a := range args {
			if strings.Contains(a, sr.fragment) {
				return sr.res, nil
			}
		}
	}
	res, ok := r.Results[base]
	if !ok {
		return platform.CommandResult{}, fmt.Errorf("fake: no result configured for %q", name)
	}
	return res, nil
}

// Run implements platform.Runner.
func (r *Runner) Run(_ context.Context, name string, args ...string) (platform.CommandResult, error) {
	return r.run(false, name, args...)
}

// RunVisible implements platform.Runner.
func (r *Runner) RunVisible(_ context.Context, name string, args ...string) (platform.CommandResult, error) {
	return r.run(true, name, args...)
}

// Calls returns the number of recorded invocations.
func (r *Runner) Calls() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.Commands)
}

// Invoked reports whether at least one invocation used the given basename.
func (r *Runner) Invoked(name string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	base := strings.ToLower(name)
	for _, c := range r.Commands {
		n := strings.ToLower(c.Name)
		if i := strings.LastIndex(n, `\`); i >= 0 {
			n = n[i+1:]
		}
		if n == base {
			return true
		}
	}
	return false
}

// System is a fixed platform.System used by tests.
type System struct {
	Sys platform.SystemInfo
	Usr platform.UserInfo
	Err error
}

// SystemInfo implements platform.System.
func (s *System) SystemInfo(context.Context) (platform.SystemInfo, error) {
	if s.Err != nil {
		return platform.SystemInfo{}, s.Err
	}
	return s.Sys, nil
}

// UserInfo implements platform.System.
func (s *System) UserInfo(context.Context) (platform.UserInfo, error) {
	if s.Err != nil {
		return platform.UserInfo{}, s.Err
	}
	return s.Usr, nil
}

// ProcessLister is a fixed platform.ProcessLister.
type ProcessLister struct {
	Names []string
	Err   error
}

// ProcessNames implements platform.ProcessLister.
func (p *ProcessLister) ProcessNames(context.Context) ([]string, error) {
	if p.Err != nil {
		return nil, p.Err
	}
	return append([]string(nil), p.Names...), nil
}
