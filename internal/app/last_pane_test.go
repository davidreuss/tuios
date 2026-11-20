package app

import (
	"testing"
	"time"

	"github.com/Gaurav-Gosain/tuios/internal/session"
	"github.com/Gaurav-Gosain/tuios/internal/terminal"
)

// lastPaneTestOS is three panes on one workspace, focus on the first.
// Focus moves in tests through FocusWindow plus reconcilePrevFocus, the pair
// Update runs for every real message.
func lastPaneTestOS(t *testing.T) *OS {
	t.Helper()
	m := newNarrowOS(t, 120, 40)
	m.SessionName = "main"
	m.WorkspaceFocus = map[int]int{}
	m.Windows = []*terminal.Window{
		{ID: "one", CustomName: "one", Width: 40, Height: 20, Workspace: 1},
		{ID: "two", CustomName: "two", Width: 40, Height: 20, Workspace: 1},
		{ID: "three", CustomName: "three", Width: 40, Height: 20, Workspace: 1},
	}
	m.FocusedWindow = 0
	m.reconcilePrevFocus()
	return m
}

// focusTo moves focus the way a message does: the jump, then the reconcile
// Update runs once a message.
func focusTo(m *OS, i int) {
	m.FocusWindow(i)
	m.reconcilePrevFocus()
}

// TestLastPaneAlternates checks the flip invariant: alternating presses walk
// between the last two panes, whatever the route focus took between them.
func TestLastPaneAlternates(t *testing.T) {
	m := lastPaneTestOS(t)

	if m.LastPane() {
		t.Fatal("nothing was focused before, yet LastPane claimed a target")
	}
	focusTo(m, 1)
	focusTo(m, 2)
	if !m.LastPane() || m.FocusedWindow != 1 {
		t.Fatalf("first flip landed on %d, want 1", m.FocusedWindow)
	}
	if !m.LastPane() || m.FocusedWindow != 2 {
		t.Fatalf("second flip landed on %d, want 2", m.FocusedWindow)
	}
}

// TestLastPaneAcrossWorkspaces checks a flip that has to cross a workspace
// switch lands on the right pane, since every jump funnels through
// FocusWindow's workspace switch.
func TestLastPaneAcrossWorkspaces(t *testing.T) {
	m := lastPaneTestOS(t)
	m.Windows[2].Workspace = 3

	focusTo(m, 2)
	if m.CurrentWorkspace != 3 {
		t.Fatalf("workspace = %d, want 3 after the jump", m.CurrentWorkspace)
	}
	if !m.LastPane() || m.FocusedWindow != 0 || m.CurrentWorkspace != 1 {
		t.Fatalf("flip back landed on pane %d workspace %d, want pane 0 workspace 1",
			m.FocusedWindow, m.CurrentWorkspace)
	}
}

// TestLastPaneDeadTarget checks a closed previous pane reports no move instead
// of landing on whatever pane reused the index.
func TestLastPaneDeadTarget(t *testing.T) {
	m := lastPaneTestOS(t)
	focusTo(m, 1)
	focusTo(m, 2)                                // previous is now pane two
	m.Windows = []*terminal.Window{m.Windows[0]} // two and three closed
	m.FocusedWindow = 0

	if m.LastPane() {
		t.Fatal("a closed target claimed to move")
	}
	if m.FocusedWindow != 0 {
		t.Fatalf("focus moved to %d on a dead target", m.FocusedWindow)
	}
}

// TestLastPaneAcrossClients is the two-client case. The daemon ran
// FocusWindow for a jump made on another client and pushed the state with the
// record of the pane focus left. This client, which was on pane two, adopts
// it: the chord flips back to the pane the OTHER client came from, not to
// this client's own previous pane, and alternating presses keep walking.
func TestLastPaneAcrossClients(t *testing.T) {
	m := lastPaneTestOS(t)
	focusTo(m, 1)

	state := &session.SessionState{
		Name:             m.SessionName,
		CurrentWorkspace: 1,
		FocusedWindowID:  "three",
		PrevFocusedID:    "one",
		PrevFocusedAt:    time.Now().UnixNano(),
		Version:          7,
		WorkspaceFocus:   map[int]string{1: "three"},
	}
	for _, w := range m.Windows {
		state.Windows = append(state.Windows, session.WindowState{
			ID: w.ID, PTYID: "pty-" + w.ID, Workspace: 1, Width: w.Width, Height: w.Height,
		})
	}
	if err := m.ApplyStateSync(state); err != nil {
		t.Fatalf("ApplyStateSync: %v", err)
	}

	if !m.LastPane() || m.FocusedWindow != 0 {
		t.Fatalf("flip after a daemon-routed jump landed on pane %d, want 0", m.FocusedWindow)
	}
	m.reconcilePrevFocus()
	if !m.LastPane() || m.FocusedWindow != 2 {
		t.Fatalf("second flip landed on pane %d, want 2", m.FocusedWindow)
	}
}

// TestLastPaneRecordSurvivesReattach checks the restore carries the record:
// the chord works on the first press of a reattached or reloaded client.
func TestLastPaneRecordSurvivesReattach(t *testing.T) {
	src := lastPaneTestOS(t)
	focusTo(src, 1)
	focusTo(src, 2)
	state := src.BuildSessionState()
	if state.PrevFocusedID != "two" {
		t.Fatalf("BuildSessionState PrevFocusedID = %q, want two", state.PrevFocusedID)
	}

	dst := NewOS(OSOptions{})
	if err := dst.RestoreFromState(state); err != nil {
		t.Fatalf("RestoreFromState: %v", err)
	}
	if dst.PrevFocusedID != "two" {
		t.Fatalf("PrevFocusedID after restore = %q, want two", dst.PrevFocusedID)
	}
}

// TestTuiosLinkTarget checks the two URI shapes the wait bar links with, and
