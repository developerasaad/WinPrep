package tui

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/developerasaad/winprep/internal/apptest"
	"github.com/developerasaad/winprep/internal/handover"
)

// press simulates a key press and returns the command it produced. The
// command is NOT executed: tests decide which commands to run, because
// commands include cursor-blink timers that must not block.
func press(t *testing.T, m *Model, key tea.KeyPressMsg) tea.Cmd {
	t.Helper()
	model, cmd := m.Update(key)
	if next, ok := model.(*Model); ok {
		*m = *next
	} else {
		t.Fatalf("Update returned %T, want *Model", model)
	}
	return cmd
}

// send delivers a message and returns the command it produced (not run).
func send(t *testing.T, m *Model, msg tea.Msg) tea.Cmd {
	t.Helper()
	model, cmd := m.Update(msg)
	if next, ok := model.(*Model); ok {
		*m = *next
	} else {
		t.Fatalf("Update returned %T, want *Model", model)
	}
	return cmd
}

// runCmd executes a command and feeds its message back, mirroring the event
// loop for commands that complete synchronously.
func runCmd(t *testing.T, m *Model, cmd tea.Cmd) {
	t.Helper()
	if cmd == nil {
		return
	}
	if msg := cmd(); msg != nil {
		send(t, m, msg)
	}
}

func newSize(w, h int) tea.WindowSizeMsg { return tea.WindowSizeMsg{Width: w, Height: h} }

func newModel(t *testing.T) (*Model, *apptest.Fixture) {
	t.Helper()
	f := apptest.New(t.TempDir())
	m := NewModel(f.App, context.Background())
	send(t, m, newSize(100, 40))
	return m, f
}

func TestMenuNavigation(t *testing.T) {
	m, _ := newModel(t)
	if m.cursor != 0 {
		t.Fatalf("cursor starts at %d, want 0", m.cursor)
	}
	press(t, m, tea.KeyPressMsg{Code: tea.KeyDown})
	press(t, m, tea.KeyPressMsg{Code: tea.KeyDown})
	if m.cursor != 2 {
		t.Errorf("cursor = %d after two downs, want 2", m.cursor)
	}
	press(t, m, tea.KeyPressMsg{Code: tea.KeyUp})
	if m.cursor != 1 {
		t.Errorf("cursor = %d after up, want 1", m.cursor)
	}
	press(t, m, tea.KeyPressMsg{Code: tea.KeyUp})
	press(t, m, tea.KeyPressMsg{Code: tea.KeyUp})
	if m.cursor != 0 {
		t.Errorf("cursor = %d after overshooting up, want 0", m.cursor)
	}
	press(t, m, tea.KeyPressMsg{Code: tea.KeyEnd})
	if m.cursor != len(menuItems)-1 {
		t.Errorf("end should jump to the last item, cursor = %d", m.cursor)
	}
	press(t, m, tea.KeyPressMsg{Code: tea.KeyHome})
	if m.cursor != 0 {
		t.Errorf("home should jump to the first item, cursor = %d", m.cursor)
	}
}

func TestMenuRendersImmediatelyWithoutHeader(t *testing.T) {
	f := apptest.New(t.TempDir())
	m := NewModel(f.App, context.Background())
	send(t, m, newSize(100, 40))
	cmd := m.Init() // like Bubble Tea: Init runs before the first render
	_ = cmd         // its result has not landed yet

	view := plain(m.View().Content)
	if !strings.Contains(view, "WinPrep") {
		t.Error("the menu must render the title")
	}
	for _, item := range menuItems {
		if !strings.Contains(view, item.label) {
			t.Errorf("menu must list %q", item.label)
		}
	}
	if !strings.Contains(view, "checking...") {
		t.Error("the header should show a loading hint before the fast load lands")
	}
	if f.Run.Calls() != 0 {
		t.Error("the header must not spawn processes; it uses registry reads only")
	}
}

func TestHeaderLoadFillsMenu(t *testing.T) {
	m, _ := newModel(t)
	runCmd(t, m, m.Init())
	if !m.headerShown {
		t.Fatal("the header result should have landed")
	}
	view := plain(m.View().Content)
	if !strings.Contains(view, "Current mode: Audit Mode confirmed") {
		t.Error("header should show the audit state")
	}
	if !strings.Contains(view, "Status: Ready for display") {
		t.Error("header should show the readiness state")
	}
}

func TestHandoverHappyPathWithDeliberateConfirm(t *testing.T) {
	m, f := newModel(t)
	// Select "Clean & Finish" (index 1).
	press(t, m, tea.KeyPressMsg{Code: tea.KeyDown})
	press(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.screen != screenHandover || m.phase != hoInspect {
		t.Fatalf("handover should be inspecting, phase=%v", m.phase)
	}

	// Deliver the preflight result (the fixture is a green machine).
	result, err := f.App.Handover.Inspect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	send(t, m, preflightLoadedMsg{loadID: m.loadID, result: result})
	if m.phase != hoReview {
		t.Fatalf("phase = %v, want hoReview", m.phase)
	}
	view := plain(m.View().Content)
	if !strings.Contains(view, "Overall: Ready") {
		t.Error("the review should show the verdict")
	}

	// Continue to confirmation.
	press(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.phase != hoConfirm {
		t.Fatalf("phase = %v, want hoConfirm", m.phase)
	}
	if m.confirmChoice != 0 {
		t.Error("confirmation must default to the safe option (go back)")
	}

	// The safe default must not start anything.
	press(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.screen != screenMenu {
		t.Error("enter on the safe default returns to the menu")
	}
	if f.Sysprep.Called {
		t.Error("the safe default must not start the reseal")
	}

	// Walk back in (cursor is still on the handover entry) and confirm
	// with the explicit Y shortcut.
	press(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})
	send(t, m, preflightLoadedMsg{loadID: m.loadID, result: result})
	press(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})
	press(t, m, tea.KeyPressMsg{Code: 'y'})

	if m.phase != hoReseal {
		t.Fatalf("phase = %v after Y, want hoReseal", m.phase)
	}
	if m.loading && m.loadingCancellable {
		t.Error("the reseal phase must not be cancellable")
	}

	// Ctrl+C is refused while Sysprep runs.
	press(t, m, tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	if m.phase != hoReseal {
		t.Error("ctrl+c must not abort the reseal phase")
	}
	if !strings.Contains(m.notice, "Sysprep is running") {
		t.Errorf("ctrl+c during reseal should explain, notice = %q", m.notice)
	}

	// Steps 4 and 6 complete.
	cleanup, cerr := f.App.Handover.CleanupArtifacts(context.Background())
	if cerr != nil {
		t.Fatal(cerr)
	}
	send(t, m, cleanupDoneMsg{loadID: m.loadID, results: cleanup})
	if m.phase != hoReseal {
		t.Fatalf("cleanup should flow into the reseal, phase = %v", m.phase)
	}
	res, rerr := f.App.Handover.Reseal(context.Background())
	if rerr != nil {
		t.Fatal(rerr)
	}
	send(t, m, resealDoneMsg{loadID: m.loadID, result: res})
	if m.phase != hoResult {
		t.Fatalf("phase = %v, want hoResult", m.phase)
	}

	view = plain(m.View().Content)
	if !strings.Contains(view, "Reseal initiated") {
		t.Error("the result screen should report the initiated reseal")
	}
	if !strings.Contains(view, "first-time setup appears") {
		t.Error("the result must ask the operator to verify the outcome")
	}
	if !f.Sysprep.Called {
		t.Error("the confirmed handover must invoke Sysprep")
	}
}

func TestHandoverBlockedRefusesToContinue(t *testing.T) {
	m, f := newModel(t)
	f.Sys.Usr.IsElevated = false // blocking condition: not elevated

	press(t, m, tea.KeyPressMsg{Code: tea.KeyDown})
	press(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})

	result, err := f.App.Handover.Inspect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.Verdict != handover.VerdictBlocked {
		t.Fatalf("fixture should be blocked, verdict = %v", result.Verdict)
	}
	send(t, m, preflightLoadedMsg{loadID: m.loadID, result: result})

	press(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.phase != hoReview {
		t.Error("a blocked handover must not reach the confirmation screen")
	}
	if f.Sysprep.Called {
		t.Error("a blocked handover must never start Sysprep")
	}
	// Scroll to the bottom so the verdict line is inside the viewport.
	press(t, m, tea.KeyPressMsg{Code: tea.KeyEnd})
	view := plain(m.View().Content)
	if !strings.Contains(view, "NOT possible") {
		t.Error("the review must state the handover is not possible")
	}
}

func TestHandoverSysprepErrorShowsLogs(t *testing.T) {
	m, f := newModel(t)
	f.Sysprep.ExitCode = 1

	press(t, m, tea.KeyPressMsg{Code: tea.KeyDown})
	press(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})
	result, err := f.App.Handover.Inspect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	send(t, m, preflightLoadedMsg{loadID: m.loadID, result: result})
	press(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})
	press(t, m, tea.KeyPressMsg{Code: 'y'})

	send(t, m, cleanupDoneMsg{loadID: m.loadID})
	res, rerr := f.App.Handover.Reseal(context.Background())
	if rerr != nil {
		t.Fatal(rerr)
	}
	send(t, m, resealDoneMsg{loadID: m.loadID, result: res})

	press(t, m, tea.KeyPressMsg{Code: tea.KeyEnd}) // ensure the tail is visible
	view := plain(m.View().Content)
	if !strings.Contains(view, "Sysprep reported an error") {
		t.Error("a failing Sysprep must be reported as an error")
	}
	if !strings.Contains(view, "setupact.log") {
		t.Error("the log paths must be shown on failure")
	}
}

func TestScrollingAndClamp(t *testing.T) {
	m, _ := newModel(t)
	// Help screen has long content.
	press(t, m, tea.KeyPressMsg{Code: tea.KeyDown})
	press(t, m, tea.KeyPressMsg{Code: tea.KeyDown})
	press(t, m, tea.KeyPressMsg{Code: tea.KeyEnter}) // Help (index 2)
	if m.screen != screenHelp {
		t.Fatalf("screen = %v, want screenHelp", m.screen)
	}
	send(t, m, newSize(80, 24)) // small viewport forces scrolling
	before := m.scrollMax()
	if before <= 0 {
		t.Fatal("help content should exceed a 24-row viewport")
	}
	press(t, m, tea.KeyPressMsg{Code: tea.KeyEnd})
	if m.scroll != before {
		t.Errorf("end should scroll to the bottom: %d, want %d", m.scroll, before)
	}
	press(t, m, tea.KeyPressMsg{Code: tea.KeyDown}) // must clamp, not overflow
	if m.scroll != before {
		t.Errorf("scroll must clamp at %d, got %d", before, m.scroll)
	}
	press(t, m, tea.KeyPressMsg{Code: tea.KeyPgUp})
	if m.scroll >= before {
		t.Errorf("pgup should move up, scroll = %d", m.scroll)
	}
	press(t, m, tea.KeyPressMsg{Code: tea.KeyEsc})
	if m.screen != screenMenu {
		t.Error("ESC should return to the menu")
	}
	if m.scroll != 0 {
		t.Error("returning to the menu should reset scrolling")
	}
}

func TestWrap(t *testing.T) {
	cases := []struct {
		name  string
		in    string
		width int
		check func([]string) bool
	}{
		{"short line", "hello world", 40, func(ls []string) bool { return len(ls) == 1 }},
		{"word wrap", "aaa bbb ccc ddd", 7, func(ls []string) bool {
			return len(ls) == 2 && ls[0] == "aaa bbb" && ls[1] == "ccc ddd"
		}},
		{"hard break", "S-1-5-21-1000000000-2000000000-3000000000-1013", 12, func(ls []string) bool {
			for _, l := range ls {
				if len(l) > 12 {
					return false
				}
			}
			return len(ls) > 1
		}},
		{"preserve breaks", "a\n\nb", 40, func(ls []string) bool {
			return len(ls) == 3 && ls[0] == "a" && ls[1] == "" && ls[2] == "b"
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if !tc.check(wrap(tc.in, tc.width)) {
				t.Errorf("wrap(%q, %d) = %v", tc.in, tc.width, wrap(tc.in, tc.width))
			}
		})
	}
}

func TestViewIsASCIIOnly(t *testing.T) {
	m, _ := newModel(t)
	runCmd(t, m, m.Init())
	view := plain(m.View().Content)
	for _, r := range view {
		if r > 0x7F {
			t.Fatalf("view contains non-ASCII rune %q (%U); Windows consoles may not render it", r, r)
		}
	}
	// Exercise the other screens' rendering too (indices 0-2; index 3 is
	// Exit, which leaves the menu rather than opening a screen).
	for i := 0; i < 3; i++ {
		press(t, m, tea.KeyPressMsg{Code: tea.KeyDown})
		press(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})
		for _, r := range plain(m.View().Content) {
			if r > 0x7F {
				t.Fatalf("screen %d contains non-ASCII rune %q", i+1, r)
			}
		}
		press(t, m, tea.KeyPressMsg{Code: tea.KeyEsc})
	}
}
