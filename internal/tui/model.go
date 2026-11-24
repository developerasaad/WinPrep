package tui

import (
	"context"

	tea "charm.land/bubbletea/v2"

	"github.com/developerasaad/winprep/internal/app"
	"github.com/developerasaad/winprep/internal/handover"
	"github.com/developerasaad/winprep/internal/persist"
)

// screen identifies the active view.
type screen int

const (
	screenMenu screen = iota
	screenHelp
	screenPersist
	screenHandover
)

// handoverPhase tracks the multi-step handover flow.
type handoverPhase int

const (
	hoInspect handoverPhase = iota // preflight running
	hoReview                       // inspection + cleanup plan shown
	hoConfirm                      // deliberate confirmation
	hoExecute                      // approved cleanup + reseal running (not cancellable)
	hoResult                       // final outcome shown
)

// menuItem is one main-menu entry.
type menuItem struct {
	label string
	// desc is the one-line explanation shown for the selected entry.
	desc string
}

var menuItems = []menuItem{
	{label: "Prepare for Display", desc: "Verify this laptop is ready to show to customers, without finishing Windows setup."},
	{label: "Clean & Finish", desc: "Remove the approved demonstration data and reseal Windows into first-time setup."},
	{label: "Help", desc: "What WinPrep does, what it deliberately does not do, and the controls."},
	{label: "Exit", desc: "Close WinPrep."},
}

// Async result messages. Each carries the load generation it belongs to;
// stale messages are dropped so a cancelled screen never shows late results.

type headerLoadedMsg struct {
	loadID int
	report persist.Report
	err    error
}

type persistLoadedMsg struct {
	loadID int
	report persist.Report
	err    error
}

type sessionLoadedMsg struct {
	loadID  int
	session *handover.Session
	err     error
}

type executeDoneMsg struct {
	loadID int
	result handover.Result
	err    error
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

	// header holds the fast readiness summary shown in the menu header.
	header      persist.Report
	headerErr   error
	headerShown bool

	// screen data
	pers    persist.Report
	persErr error

	// scroll position for long views; view() recomputes the maximum.
	scroll int

	// handover flow
	phase         handoverPhase
	session       *handover.Session
	sessionErr    error
	confirmChoice int // 0 = Cancel (safe default), 1 = proceed
	result        handover.Result
	resultErr     error

	// notice is a one-line message shown above the footer (errors, results).
	notice string
}

// NewModel returns the initial model.
func NewModel(a *app.App, parent context.Context) *Model {
	return &Model{
		a:      a,
		parent: parent,
		screen: screenMenu,
	}
}

// Init starts the fast header load. Nothing else is queried at startup; the
// menu renders immediately.
func (m *Model) Init() tea.Cmd {
	return m.start(func(ctx context.Context) tea.Msg {
		r, err := m.a.Persist.Inspect(ctx)
		return headerLoadedMsg{report: r, err: err}
	}, "checking this laptop", false)
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
		case persistLoadedMsg:
			v.loadID = id
			return v
		case sessionLoadedMsg:
			v.loadID = id
			return v
		case executeDoneMsg:
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

	case persistLoadedMsg:
		if m.stale(msg.loadID) {
			return m, nil
		}
		m.finishLoading()
		m.pers, m.persErr = msg.report, msg.err
		m.scroll = 0
		return m, nil

	case sessionLoadedMsg:
		if m.stale(msg.loadID) {
			return m, nil
		}
		m.finishLoading()
		m.session, m.sessionErr = msg.session, msg.err
		m.phase = hoReview
		m.scroll = 0
		return m, nil

	case executeDoneMsg:
		if m.stale(msg.loadID) {
			return m, nil
		}
		m.finishLoading()
		m.result, m.resultErr = msg.result, msg.err
		m.phase = hoResult
		m.scroll = 0
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
		if m.screen == screenHandover && m.phase == hoExecute {
			m.notice = "The cleanup and reseal are running; WinPrep stays open until they finish."
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

	switch m.screen {
	case screenMenu:
		return m.menuKey(s)
	case screenHelp:
		return m.viewerKey(s, screenMenu)
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
		case 0: // Prepare for Display
			m.screen = screenPersist
			return m, m.start(func(ctx context.Context) tea.Msg {
				r, err := m.a.Persist.Inspect(ctx)
				return persistLoadedMsg{report: r, err: err}
			}, "checking this laptop", true)
		case 1: // Clean & Finish
			m.screen = screenHandover
			m.phase = hoInspect
			m.confirmChoice = 0
			m.session, m.sessionErr = nil, nil
			m.result, m.resultErr = handover.Result{}, nil
			return m, m.start(func(ctx context.Context) tea.Msg {
				s, err := m.a.Handover.Begin(ctx)
				return sessionLoadedMsg{session: s, err: err}
			}, "inspecting this laptop and planning the cleanup", true)
		case 2: // Help
			m.screen = screenHelp
			m.scroll = 0
		case 3: // Exit
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

func (m *Model) handoverKey(s string) (tea.Model, tea.Cmd) {
	switch m.phase {
	case hoReview:
		switch s {
		case "enter":
			if m.session == nil || m.session.Preflight.Verdict == handover.VerdictBlocked {
				m.notice = "The handover cannot proceed; resolve the issues above."
				return m, nil
			}
			m.phase = hoConfirm
			m.confirmChoice = 0 // safe default: cancel
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
			return m.beginExecution()
		case "enter":
			if m.confirmChoice == 1 {
				return m.beginExecution()
			}
			m.screen = screenMenu
			m.notice = "Cancelled. Nothing was changed."
		}
	case hoResult:
		return m.viewerKey(s, screenMenu)
	}
	return m, nil
}

// beginExecution approves the reviewed plan and runs the one approved
// sequence through the shared controller: revalidate, clean, verify, reseal.
// Once started it runs to completion; the controller ignores interrupts for
// exactly this window.
func (m *Model) beginExecution() (tea.Model, tea.Cmd) {
	m.phase = hoExecute
	m.scroll = 0
	m.session.Approve()
	return m, m.start(func(ctx context.Context) tea.Msg {
		res, err := m.session.Execute(ctx)
		return executeDoneMsg{result: res, err: err}
	}, "removing the approved demonstration data and resealing Windows (do not turn off the laptop)", false)
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
	case screenPersist:
		if m.persErr != nil {
			return wrap("Could not check this laptop:\n"+m.persErr.Error(), w)
		}
		return wrapLines(m.pers.Lines(), w)
	case screenHandover:
		switch m.phase {
		case hoReview:
			return wrapLines(renderSessionReview(m.session, m.sessionErr), w)
		case hoResult:
			return wrapLines(renderResult(m.result, m.resultErr), w)
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
