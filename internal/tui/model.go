package tui

import (
	"context"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/developerasaad/winprep/internal/app"
	"github.com/developerasaad/winprep/internal/diagnostics"
	"github.com/developerasaad/winprep/internal/handover"
	"github.com/developerasaad/winprep/internal/persist"
	"github.com/developerasaad/winprep/internal/status"
)

// screen identifies the active view.
type screen int

const (
	screenMenu screen = iota
	screenHelp
	screenStatus
	screenCheck
	screenPersist
	screenHandover
)

// handoverPhase tracks the multi-step handover flow.
type handoverPhase int

const (
	hoInspect handoverPhase = iota // preflight running
	hoReview                       // preflight result shown
	hoConfirm                      // deliberate confirmation
	hoReseal                       // cleanup + Sysprep running (not cancellable)
	hoResult                       // final outcome shown
)

// menuItem is one main-menu entry.
type menuItem struct {
	label string
	// desc is the one-line explanation shown for the selected entry.
	desc string
}

var menuItems = []menuItem{
	{label: "Prepare for display", desc: "Verify this laptop is ready to show to customers, without finishing Windows setup."},
	{label: "Check this laptop", desc: "Read-only health check: computer, storage, battery, devices, activation."},
	{label: "Ready for buyer", desc: "Inspect, clean up WinPrep's own changes, and reseal Windows into first-time setup."},
	{label: "System information", desc: "Windows edition, build, user, privileges, and setup state."},
	{label: "Help", desc: "What WinPrep does, what it deliberately does not do, and the controls."},
	{label: "Exit", desc: "Close WinPrep."},
}

// Async result messages. Each carries the load generation it belongs to;
// stale messages are dropped so a cancelled screen never shows late results.

type headerLoadedMsg struct {
	loadID int
	report status.Report
	err    error
}

type statusLoadedMsg struct {
	loadID int
	report status.Report
	err    error
}

type persistLoadedMsg struct {
	loadID int
	report persist.Report
	err    error
}

type checkLoadedMsg struct {
	loadID int
	report diagnostics.Report
	err    error
}

type preflightLoadedMsg struct {
	loadID int
	result handover.PreflightResult
	err    error
}

type cleanupDoneMsg struct {
	loadID  int
	results []handover.CleanupResult
	err     error
}

type resealDoneMsg struct {
	loadID int
	result handover.ResealResult
	err    error
}

type batteryDoneMsg struct {
	loadID int
	path   string
	err    error
}

type exportDoneMsg struct {
	path string
	err  error
}

// Model is the Bubble Tea model driving the whole interface. All methods use
// pointer receivers so state changes inside Init and the key handlers persist
// across Update calls.
type Model struct {
	a *app.App
	// parent is the program-lifetime context; per-operation cancel contexts
	// derive from it.
	parent context.Context

	width, height int

	screen screen
	cursor int

	// loadID is the generation counter for async operations.
	loadID int
	// loading is true while an async operation runs.
	loading bool
	// loadingCancellable is false once the operation must not be interrupted
	// (the post-confirmation handover steps).
	loadingCancellable bool
	loadingWhat        string
	cancel             context.CancelFunc

	// header holds the fast status summary shown in the menu header.
	header      status.Report
	headerErr   error
	headerShown bool

	// screen data
	stat    status.Report
	statErr error
	pers    persist.Report
	persErr error
	diag    diagnostics.Report
	diagErr error

	// scroll position for long views; view() recomputes the maximum.
	scroll int

	// handover flow
	phase         handoverPhase
	preflight     handover.PreflightResult
	preflightErr  error
	confirmChoice int // 0 = go back (safe default), 1 = proceed
	cleanup       []handover.CleanupResult
	cleanupErr    error
	reseal        handover.ResealResult
	resealErr     error

	// check-screen extras
	exporting   bool
	exportInput textinput.Model
	exportErr   string
	batteryNote string

	// notice is a one-line message shown above the footer (errors, results).
	notice string
}

// NewModel returns the initial model.
func NewModel(a *app.App, parent context.Context) *Model {
	ti := textinput.New()
	ti.Placeholder = "report.txt"
	ti.CharLimit = 260
	ti.Prompt = "Save to: "
	return &Model{
		a:           a,
		parent:      parent,
		screen:      screenMenu,
		exportInput: ti,
	}
}

// Init starts the fast header load. Nothing else is queried at startup; the
// menu renders immediately.
func (m *Model) Init() tea.Cmd {
	return m.start(func(ctx context.Context) tea.Msg {
		r, err := m.a.Status.Inspect(ctx)
		return headerLoadedMsg{report: r, err: err}
	}, "checking this computer", false)
}

// start registers an async operation. cancellable marks whether the operator
// may back out with ESC; it is false for the post-confirmation handover
// steps, which must not be interrupted once accepted.
func (m *Model) start(fn func(ctx context.Context) tea.Msg, what string, cancellable bool) tea.Cmd {
	if m.cancel != nil {
		m.cancel()
	}
	m.loadID++
	m.loading = true
	m.loadingCancellable = cancellable
	m.loadingWhat = what
	m.scroll = 0
	m.notice = ""
	id := m.loadID
	ctx, cancel := context.WithCancel(m.parent)
	m.cancel = cancel
	return func() tea.Msg {
		defer cancel()
		msg := fn(ctx)
		switch v := msg.(type) {
		case headerLoadedMsg:
			v.loadID = id
			return v
		case statusLoadedMsg:
			v.loadID = id
			return v
		case persistLoadedMsg:
			v.loadID = id
			return v
		case checkLoadedMsg:
			v.loadID = id
			return v
		case preflightLoadedMsg:
			v.loadID = id
			return v
		case cleanupDoneMsg:
			v.loadID = id
			return v
		case resealDoneMsg:
			v.loadID = id
			return v
		case batteryDoneMsg:
			v.loadID = id
			return v
		default:
			return v
		}
	}
}

// stale reports whether a message belongs to an operation that was cancelled
// or superseded.
func (m *Model) stale(id int) bool { return id != m.loadID }

// finishLoading marks loading done.
func (m *Model) finishLoading() {
	m.loading = false
	m.loadingWhat = ""
	if m.cancel != nil {
		m.cancel()
		m.cancel = nil
	}
}

// cancelLoad cancels the running operation, if any, and invalidates its
// future result.
func (m *Model) cancelLoad() {
	if m.cancel != nil {
		m.cancel()
		m.cancel = nil
	}
	m.loadID++ // invalidate any in-flight result
	m.loading = false
	m.loadingWhat = ""
}

// Update handles messages for the whole application.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.clampScroll()
		return m, nil

	case tea.KeyPressMsg:
		return m.key(msg)

	case headerLoadedMsg:
		if m.stale(msg.loadID) {
			return m, nil
		}
		m.finishLoading()
		m.header, m.headerErr = msg.report, msg.err
		m.headerShown = true
		return m, nil

	case statusLoadedMsg:
		if m.stale(msg.loadID) {
			return m, nil
		}
		m.finishLoading()
		m.stat, m.statErr = msg.report, msg.err
		m.scroll = 0
		return m, nil

	case persistLoadedMsg:
		if m.stale(msg.loadID) {
			return m, nil
		}
		m.finishLoading()
		m.pers, m.persErr = msg.report, msg.err
		m.scroll = 0
		return m, nil

	case checkLoadedMsg:
		if m.stale(msg.loadID) {
			return m, nil
		}
		m.finishLoading()
		m.diag, m.diagErr = msg.report, msg.err
		m.scroll = 0
		return m, nil

	case preflightLoadedMsg:
		if m.stale(msg.loadID) {
			return m, nil
		}
		m.finishLoading()
		m.preflight, m.preflightErr = msg.result, msg.err
		m.phase = hoReview
		m.scroll = 0
		return m, nil

	case cleanupDoneMsg:
		if m.stale(msg.loadID) {
			return m, nil
		}
		m.cleanup, m.cleanupErr = msg.results, msg.err
		if m.cleanupErr != nil {
			// Step 4 failed, so the reseal must not start (step 6 rules).
			m.finishLoading()
			m.phase = hoResult
			m.reseal, m.resealErr = handover.ResealResult{}, nil
			m.scroll = 0
			return m, nil
		}
		return m, m.start(func(ctx context.Context) tea.Msg {
			r, err := m.a.Handover.Reseal(ctx)
			return resealDoneMsg{result: r, err: err}
		}, "running Sysprep (do not turn off the laptop)", false)

	case resealDoneMsg:
		if m.stale(msg.loadID) {
			return m, nil
		}
		m.finishLoading()
		m.reseal, m.resealErr = msg.result, msg.err
		m.phase = hoResult
		m.scroll = 0
		return m, nil

	case batteryDoneMsg:
		if m.stale(msg.loadID) {
			return m, nil
		}
		m.finishLoading()
		if msg.err != nil {
			m.batteryNote = "Battery report failed: " + msg.err.Error()
		} else {
			m.batteryNote = "Battery report saved to: " + msg.path
		}
		return m, nil

	case exportDoneMsg:
		m.exporting = false
		m.exportInput.Blur()
		if msg.err != nil {
			m.exportErr = "Saving failed: " + msg.err.Error()
		} else {
			m.exportErr = ""
			m.notice = "Report saved to: " + msg.path
		}
		return m, nil
	}
	return m, nil
}

// key handles a key press according to the active screen and phase.
func (m *Model) key(k tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	s := k.String()

	// Ctrl+C quits unless the handover is mid-run; leaving then would suggest
	// the reseal itself was cancelled, which it cannot be.
	if s == "ctrl+c" {
		if m.screen == screenHandover && m.phase == hoReseal {
			m.notice = "Sysprep is running; WinPrep stays open until it finishes."
			return m, nil
		}
		m.cancelLoad()
		return m, tea.Quit
	}

	// While an operation runs, the screen shows a working line. Cancellable
	// (read-only) operations back out on ESC; the post-confirmation handover
	// steps ignore input until they finish.
	if m.loading {
		if s == "esc" {
			if m.loadingCancellable {
				m.cancelLoad()
				m.screen = screenMenu
				m.notice = "Cancelled."
			} else {
				m.notice = m.loadingWhat + " - please wait."
			}
		}
		return m, nil
	}

	if m.exporting {
		return m.exportKey(s, k)
	}

	switch m.screen {
	case screenMenu:
		return m.menuKey(s)
	case screenHelp:
		return m.viewerKey(s, screenMenu)
	case screenStatus:
		return m.viewerKey(s, screenMenu)
	case screenCheck:
		return m.checkKey(s)
	case screenPersist:
		return m.viewerKey(s, screenMenu)
	case screenHandover:
		return m.handoverKey(s)
	}
	return m, nil
}

func (m *Model) menuKey(s string) (tea.Model, tea.Cmd) {
	switch s {
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < len(menuItems)-1 {
			m.cursor++
		}
	case "home":
		m.cursor = 0
	case "end":
		m.cursor = len(menuItems) - 1
	case "enter":
		switch m.cursor {
		case 0: // Prepare for display
			m.screen = screenPersist
			return m, m.start(func(ctx context.Context) tea.Msg {
				r, err := m.a.Persist.Inspect(ctx)
				return persistLoadedMsg{report: r, err: err}
			}, "checking this laptop", true)
		case 1: // Check this laptop
			m.screen = screenCheck
			m.batteryNote = ""
			return m, m.start(func(ctx context.Context) tea.Msg {
				r, err := m.a.Diag.Inspect(ctx)
				return checkLoadedMsg{report: r, err: err}
			}, "checking this laptop (storage, battery, devices)", true)
		case 2: // Ready for buyer
			m.screen = screenHandover
			m.phase = hoInspect
			m.confirmChoice = 0
			m.cleanup, m.cleanupErr = nil, nil
			m.reseal, m.resealErr = handover.ResealResult{}, nil
			return m, m.start(func(ctx context.Context) tea.Msg {
				r, err := m.a.Handover.Inspect(ctx)
				return preflightLoadedMsg{result: r, err: err}
			}, "inspecting this laptop before the handover", true)
		case 3: // System information
			m.screen = screenStatus
			return m, m.start(func(ctx context.Context) tea.Msg {
				r, err := m.a.Status.Inspect(ctx)
				return statusLoadedMsg{report: r, err: err}
			}, "reading system information", true)
		case 4: // Help
			m.screen = screenHelp
			m.scroll = 0
		case 5: // Exit
			return m, tea.Quit
		}
	case "esc", "q":
		return m, tea.Quit
	}
	return m, nil
}

// viewerKey handles scrollable read-only screens. back is the screen ESC
// returns to.
func (m *Model) viewerKey(s string, back screen) (tea.Model, tea.Cmd) {
	switch s {
	case "up", "k":
		m.scroll = maxInt(0, m.scroll-1)
	case "down", "j":
		m.scroll++
	case "pgup", "b":
		m.scroll = maxInt(0, m.scroll-m.viewportHeight())
	case "pgdown", "space", "f":
		m.scroll += m.viewportHeight()
	case "home":
		m.scroll = 0
	case "end", "shift+g":
		m.scroll = m.scrollMax()
	case "esc", "q", "enter":
		m.screen = back
		m.scroll = 0
		m.notice = ""
	}
	m.clampScroll()
	return m, nil
}

func (m *Model) checkKey(s string) (tea.Model, tea.Cmd) {
	switch s {
	case "e", "E":
		m.exporting = true
		m.exportErr = ""
		m.exportInput.SetValue(defaultExportPath("winprep-report.txt"))
		m.exportInput.CursorEnd()
		return m, m.exportInput.Focus()
	case "b", "B":
		m.batteryNote = ""
		return m, m.start(func(ctx context.Context) tea.Msg {
			p, err := m.a.Diag.BatteryReport(ctx, "")
			return batteryDoneMsg{path: p, err: err}
		}, "generating the battery report", true)
	}
	return m.viewerKey(s, screenMenu)
}

// exportKey handles the save-path prompt on the check screen.
func (m *Model) exportKey(s string, k tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch s {
	case "enter":
		path := m.exportInput.Value()
		if path == "" {
			m.exportErr = "Enter a file path, or press ESC to cancel."
			return m, nil
		}
		text := m.diag.Text()
		return m, func() tea.Msg {
			err := writeReport(path, text)
			return exportDoneMsg{path: path, err: err}
		}
	case "esc":
		m.exporting = false
		m.exportInput.Blur()
		m.exportErr = ""
		return m, nil
	}
	var cmd tea.Cmd
	m.exportInput, cmd = m.exportInput.Update(k)
	return m, cmd
}

func (m *Model) handoverKey(s string) (tea.Model, tea.Cmd) {
	switch m.phase {
	case hoReview:
		switch s {
		case "enter":
			if m.preflight.Verdict == handover.VerdictBlocked {
				m.notice = "The handover cannot proceed; resolve the issues above."
				return m, nil
			}
			m.phase = hoConfirm
			m.confirmChoice = 0 // safe default: go back
			m.scroll = 0
			return m, nil
		default:
			return m.viewerKey(s, screenMenu)
		}
	case hoConfirm:
		switch s {
		case "left", "right", "up", "down", "tab", "shift+tab":
			m.confirmChoice = 1 - m.confirmChoice
		case "home":
			m.confirmChoice = 0
		case "n", "N", "esc", "q":
			m.screen = screenMenu
			m.notice = "Cancelled. Nothing was changed."
		case "y", "Y":
			return m.beginReseal()
		case "enter":
			if m.confirmChoice == 1 {
				return m.beginReseal()
			}
			m.screen = screenMenu
			m.notice = "Cancelled. Nothing was changed."
		}
	case hoResult:
		return m.viewerKey(s, screenMenu)
	}
	return m, nil
}

// beginReseal starts step 4 (cleanup of WinPrep's own artifacts) followed by
// step 6 (Sysprep). It only runs after a deliberate confirmation, and once
// started it runs to completion.
func (m *Model) beginReseal() (tea.Model, tea.Cmd) {
	m.phase = hoReseal
	m.scroll = 0
	return m, m.start(func(ctx context.Context) tea.Msg {
		results, err := m.a.Handover.CleanupArtifacts(ctx)
		return cleanupDoneMsg{results: results, err: err}
	}, "removing WinPrep's own artifacts", false)
}

// viewportHeight is the number of content rows available for scrollable
// screens: total height minus chrome (title, footer, padding).
func (m *Model) viewportHeight() int {
	h := m.height - 10
	if h < 3 {
		h = 3
	}
	return h
}

func (m *Model) scrollMax() int {
	return maxInt(0, len(m.viewportLines())-m.viewportHeight())
}

func (m *Model) clampScroll() {
	if m.scroll > m.scrollMax() {
		m.scroll = m.scrollMax()
	}
	if m.scroll < 0 {
		m.scroll = 0
	}
}

// viewportLines returns the wrapped content lines of the active scrollable
// screen. The menu and confirmation screens do not scroll.
func (m *Model) viewportLines() []string {
	w := m.wrapWidth()
	switch m.screen {
	case screenHelp:
		return wrap(helpText(), w)
	case screenStatus:
		if m.statErr != nil {
			return wrap("Could not read the system status:\n"+m.statErr.Error(), w)
		}
		return wrapLines(m.stat.Lines(), w)
	case screenPersist:
		if m.persErr != nil {
			return wrap("Could not check this laptop:\n"+m.persErr.Error(), w)
		}
		return wrapLines(m.pers.Lines(), w)
	case screenCheck:
		return wrap(renderDiagnostics(m.diag, m.diagErr, m.batteryNote), w)
	case screenHandover:
		switch m.phase {
		case hoReview:
			return wrapLines(renderPreflight(m.preflight, m.preflightErr), w)
		case hoResult:
			return wrapLines(renderOutcome(m.cleanup, m.cleanupErr, m.reseal, m.resealErr), w)
		}
	}
	return nil
}

// wrapWidth is the usable text width for content.
func (m *Model) wrapWidth() int {
	w := contentWidth
	if m.width > 0 && m.width-4 < w {
		w = m.width - 4
	}
	if w < 20 {
		w = 20
	}
	return w
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
