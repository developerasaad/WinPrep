package app

import (
	"github.com/developerasaad/winprep/internal/artifacts"
	"github.com/developerasaad/winprep/internal/audit"
	"github.com/developerasaad/winprep/internal/handover"
	"github.com/developerasaad/winprep/internal/persist"
	"github.com/developerasaad/winprep/internal/platform"
)

// App is the assembled application. Every field is a small service with a
// single responsibility; the presentation layers orchestrate them but never
// implement Windows behavior themselves.
type App struct {
	Audit    *audit.Service
	Persist  *persist.Service
	Handover *handover.Controller
}

// Assemble builds the domain services from the given platform pieces. It is
// exported so tests (and any future non-Windows assembly) can construct a
// fully wired App from fake or alternative implementations.
func Assemble(sys platform.System, reg platform.Registry, run platform.Runner, procs platform.ProcessLister,
	store *artifacts.Store, sysprep handover.SysprepRunner, sysprepReports platform.SysprepMonitor,
	lock platform.InstanceLock, usersDir string, stateIni platform.SetupStateFile) *App {

	a := &App{
		Audit: audit.NewService(reg, sys, stateIni),
	}
	a.Persist = persist.NewService(sys, a.Audit, procs, store)
	a.Handover = &handover.Controller{
		Sys:            sys,
		Reg:            reg,
		Procs:          procs,
		Audit:          a.Audit,
		Store:          store,
		Sysprep:        sysprep,
		SysprepReports: sysprepReports,
		Lock:           lock,
		UsersDir:       usersDir,
	}
	return a
}
