package app

import (
	"github.com/developerasaad/winprep/internal/artifacts"
	"github.com/developerasaad/winprep/internal/audit"
	"github.com/developerasaad/winprep/internal/diagnostics"
	"github.com/developerasaad/winprep/internal/handover"
	"github.com/developerasaad/winprep/internal/persist"
	"github.com/developerasaad/winprep/internal/platform"
	"github.com/developerasaad/winprep/internal/status"
)

// App is the assembled application. Every field is a small service with a
// single responsibility; the presentation layers orchestrate them but never
// implement Windows behavior themselves.
type App struct {
	System   platform.System
	Registry platform.Registry
	Runner   platform.Runner
	Procs    platform.ProcessLister

	Status   *status.Service
	Audit    *audit.Service
	Diag     *diagnostics.Service
	Persist  *persist.Service
	Handover *handover.Flow
}

// Assemble builds the domain services from the given platform pieces. It is
// exported so tests (and any future non-Windows assembly) can construct a
// fully wired App from fake or alternative implementations.
func Assemble(sys platform.System, reg platform.Registry, run platform.Runner, procs platform.ProcessLister,
	store *artifacts.Store, psPath, powerCfgPath, sysprepPath string, sysprep handover.SysprepRunner, usersDir string) *App {

	a := &App{
		System:   sys,
		Registry: reg,
		Runner:   run,
		Procs:    procs,
		Audit:    audit.NewService(reg, sys),
	}
	a.Status = status.NewService(sys, a.Audit)
	a.Diag = diagnostics.NewService(sys, run, psPath, powerCfgPath)
	a.Persist = persist.NewService(sys, a.Audit, procs, store)
	a.Handover = &handover.Flow{
		Sys:      sys,
		Reg:      reg,
		Run:      run,
		Procs:    procs,
		Audit:    a.Audit,
		Store:    store,
		Sysprep:  sysprep,
		PSPath:   psPath,
		UsersDir: usersDir,
	}
	return a
}
